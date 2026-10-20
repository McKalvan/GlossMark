//! Scan events from depot and handheld scanners. The barcode on the shipping
//! label is a Code 128 encoding of the tracking number: `KF`, ten digits and
//! a Luhn check digit, e.g. `KF12345678903`.

use std::fmt;

use chrono::{DateTime, Utc};
use serde::Deserialize;
use sqlx::PgPool;
use uuid::Uuid;

use crate::delivery::confirmation::{record_proof_of_delivery, ProofOfDelivery};
use crate::delivery::geofence::Coordinate;
use crate::error::{AppError, AppResult};
use crate::events::publisher::EventPublisher;
use crate::events::types::{Proof, ProofKind};
use crate::models::{EventKind, TrackedShipment};

pub const TRACKING_NUMBER_PREFIX: &str = "KF";
pub const TRACKING_NUMBER_LEN: usize = 13;

/// AIM symbology identifiers a scanner may prepend to Code 128 data.
const CODE128_SYMBOLOGY_IDS: [&str; 4] = ["]C0", "]C1", "]C2", "]C4"];
/// FNC1 is transmitted as ASCII GS by most scanners.
const GS: char = '\u{1d}';

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct TrackingNumber(String);

impl TrackingNumber {
    pub fn as_str(&self) -> &str {
        &self.0
    }
}

impl fmt::Display for TrackingNumber {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(&self.0)
    }
}

#[derive(Debug, Clone, PartialEq, Eq, thiserror::Error)]
pub enum ScanParseError {
    #[error("barcode is empty")]
    Empty,
    #[error("tracking number must be 13 characters, got {0}")]
    WrongLength(usize),
    #[error("tracking number must start with KF")]
    WrongPrefix,
    #[error("tracking number must have 11 digits after the prefix")]
    NonDigit,
    #[error("check digit mismatch: expected {expected}, found {found}")]
    CheckDigitMismatch { expected: u32, found: u32 },
}

/// Luhn check digit for a string of ASCII digits; `None` if any non-digit.
pub fn luhn_check_digit(payload: &str) -> Option<u32> {
    let mut sum = 0;
    for (i, c) in payload.chars().rev().enumerate() {
        let mut d = c.to_digit(10)?;
        if i % 2 == 0 {
            d *= 2;
            if d > 9 {
                d -= 9;
            }
        }
        sum += d;
    }
    Some((10 - sum % 10) % 10)
}

/// Extracts and validates the tracking number from raw Code 128 scan data.
pub fn parse_tracking_number(raw: &str) -> Result<TrackingNumber, ScanParseError> {
    let mut data = raw.trim();
    for id in CODE128_SYMBOLOGY_IDS {
        if let Some(rest) = data.strip_prefix(id) {
            data = rest;
            break;
        }
    }
    let cleaned: String = data
        .chars()
        .filter(|c| *c != GS && !c.is_whitespace())
        .collect::<String>()
        .to_ascii_uppercase();

    if cleaned.is_empty() {
        return Err(ScanParseError::Empty);
    }
    if cleaned.len() != TRACKING_NUMBER_LEN {
        return Err(ScanParseError::WrongLength(cleaned.chars().count()));
    }
    let Some(digits) = cleaned.strip_prefix(TRACKING_NUMBER_PREFIX) else {
        return Err(ScanParseError::WrongPrefix);
    };
    if !digits.bytes().all(|b| b.is_ascii_digit()) {
        return Err(ScanParseError::NonDigit);
    }

    let (payload, check) = digits.split_at(digits.len() - 1);
    let found = check.parse::<u32>().map_err(|_| ScanParseError::NonDigit)?;
    let expected = luhn_check_digit(payload).ok_or(ScanParseError::NonDigit)?;
    if expected != found {
        return Err(ScanParseError::CheckDigitMismatch { expected, found });
    }

    Ok(TrackingNumber(cleaned))
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum ScanType {
    DepotIn,
    DepotOut,
    OutForDelivery,
    Delivered,
    Exception,
}

impl ScanType {
    pub fn as_str(self) -> &'static str {
        match self {
            ScanType::DepotIn => "DEPOT_IN",
            ScanType::DepotOut => "DEPOT_OUT",
            ScanType::OutForDelivery => "OUT_FOR_DELIVERY",
            ScanType::Delivered => "DELIVERED",
            ScanType::Exception => "EXCEPTION",
        }
    }
}

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ScanInput {
    pub device_id: String,
    pub barcode: String,
    pub scan_type: ScanType,
    pub location: Option<String>,
    pub driver_id: Option<Uuid>,
    pub lat: Option<f64>,
    pub lng: Option<f64>,
    pub scanned_at: DateTime<Utc>,
    /// Signature or photo captured on the handheld for DELIVERED scans.
    pub proof: Option<Proof>,
}

#[derive(Debug, Clone)]
pub struct ScanOutcome {
    pub event_id: i64,
    pub tracking_number: TrackingNumber,
    pub shipment_id: Option<Uuid>,
    pub delivered: bool,
}

// @gloss implements .ScanIngestor conf=0.45
pub async fn ingest_scan(
    pool: &PgPool,
    publisher: &EventPublisher,
    scan: ScanInput,
) -> AppResult<ScanOutcome> {
    let tracking_number = parse_tracking_number(&scan.barcode)?;

    let shipment = sqlx::query_as::<_, TrackedShipment>(
        "SELECT * FROM tracked_shipments WHERE tracking_number = $1",
    )
    .bind(tracking_number.as_str())
    .fetch_optional(pool)
    .await?;
    let shipment_id = shipment.as_ref().map(|s| s.shipment_id);

    let (event_id,): (i64,) = sqlx::query_as(
        "INSERT INTO tracking_events \
            (kind, shipment_id, tracking_number, driver_id, device_id, lat, lng, scan_type, scan_location, raw_barcode, recorded_at) \
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) \
         ON CONFLICT (device_id, kind, recorded_at) DO UPDATE SET received_at = tracking_events.received_at \
         RETURNING id",
    )
    .bind(EventKind::Scan.as_str())
    .bind(shipment_id)
    .bind(tracking_number.as_str())
    .bind(scan.driver_id)
    .bind(&scan.device_id)
    .bind(scan.lat)
    .bind(scan.lng)
    .bind(scan.scan_type.as_str())
    .bind(scan.location.as_deref())
    .bind(&scan.barcode)
    .bind(scan.scanned_at)
    .fetch_one(pool)
    .await?;

    let mut delivered = false;
    if scan.scan_type == ScanType::Delivered {
        let shipment = shipment.ok_or_else(|| {
            AppError::NotFound(format!("tracked shipment {tracking_number}"))
        })?;
        let pod = proof_from_scan(&scan, &shipment)?;
        record_proof_of_delivery(pool, publisher, shipment.shipment_id, pod).await?;
        delivered = true;
    }

    Ok(ScanOutcome {
        event_id,
        tracking_number,
        shipment_id,
        delivered,
    })
}

/// A DELIVERED scan carries a signature or photo reference; without one the
/// handheld's position is offered as geofence proof.
fn proof_from_scan(scan: &ScanInput, shipment: &TrackedShipment) -> AppResult<ProofOfDelivery> {
    let position = match (scan.lat, scan.lng, shipment.last_lat, shipment.last_lng) {
        (Some(lat), Some(lng), _, _) => Coordinate::new(lat, lng),
        (_, _, Some(lat), Some(lng)) => Coordinate::new(lat, lng),
        _ => {
            return Err(AppError::Validation(
                "DELIVERED scan needs a position when no ping has been received".to_owned(),
            ))
        }
    };

    let proof = match &scan.proof {
        Some(proof) => proof.clone(),
        None => Proof {
            kind: ProofKind::Geofence,
            reference: format!("geofence:{}:{}", scan.device_id, scan.scanned_at.to_rfc3339()),
        },
    };

    Ok(ProofOfDelivery {
        delivered_at: scan.scanned_at,
        position,
        proof,
    })
}
