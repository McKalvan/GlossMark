//! GPS pings from driver devices: validate, store, then run the geofence
//! check against the driver's open stops.

use chrono::{DateTime, Duration, Utc};
use serde::{Deserialize, Serialize};
use sqlx::PgPool;
use uuid::Uuid;

use crate::delivery::geofence::{detect_arrival, Coordinate};
use crate::error::AppResult;
use crate::models::{EventKind, ShipmentStatus};

/// Pings with a reported accuracy worse than this are dropped outright.
pub const MAX_PING_ACCURACY_M: f32 = 500.0;
/// Devices buffer pings while offline; anything older is not worth storing.
pub const MAX_PING_AGE_HOURS: i64 = 24;
/// Tolerated device clock drift into the future.
pub const MAX_CLOCK_SKEW_SECS: i64 = 120;

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PingInput {
    pub device_id: String,
    pub driver_id: Uuid,
    pub lat: f64,
    pub lng: f64,
    pub accuracy_m: Option<f32>,
    pub recorded_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum PingRejection {
    InvalidCoordinate,
    PoorAccuracy,
    TooOld,
    InFuture,
    MissingDevice,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RejectedPing {
    pub index: usize,
    pub reason: PingRejection,
}

#[derive(Debug, Default, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PingBatchResult {
    pub accepted: usize,
    pub duplicates: usize,
    pub rejected: Vec<RejectedPing>,
    pub arrivals: Vec<Uuid>,
}

pub fn validate_ping(ping: &PingInput, now: DateTime<Utc>) -> Result<(), PingRejection> {
    if ping.device_id.trim().is_empty() {
        return Err(PingRejection::MissingDevice);
    }
    if !Coordinate::new(ping.lat, ping.lng).is_valid() {
        return Err(PingRejection::InvalidCoordinate);
    }
    if ping
        .accuracy_m
        .is_some_and(|acc| !(0.0..=MAX_PING_ACCURACY_M).contains(&acc))
    {
        return Err(PingRejection::PoorAccuracy);
    }
    if ping.recorded_at > now + Duration::seconds(MAX_CLOCK_SKEW_SECS) {
        return Err(PingRejection::InFuture);
    }
    if ping.recorded_at < now - Duration::hours(MAX_PING_AGE_HOURS) {
        return Err(PingRejection::TooOld);
    }
    Ok(())
}

// @gloss implements .PingIngestor conf=0.9
pub async fn ingest_batch(pool: &PgPool, pings: Vec<PingInput>) -> AppResult<PingBatchResult> {
    let now = Utc::now();
    let mut result = PingBatchResult::default();

    for (index, ping) in pings.into_iter().enumerate() {
        if let Err(reason) = validate_ping(&ping, now) {
            result.rejected.push(RejectedPing { index, reason });
            continue;
        }
        if !store_ping(pool, &ping).await? {
            result.duplicates += 1;
            continue;
        }
        result.accepted += 1;
        update_last_position(pool, &ping).await?;
        result.arrivals.extend(check_arrivals(pool, &ping).await?);
    }

    Ok(result)
}

async fn store_ping(pool: &PgPool, ping: &PingInput) -> AppResult<bool> {
    let inserted = sqlx::query(
        "INSERT INTO tracking_events (kind, driver_id, device_id, lat, lng, accuracy_m, recorded_at) \
         VALUES ($1, $2, $3, $4, $5, $6, $7) \
         ON CONFLICT (device_id, kind, recorded_at) DO NOTHING",
    )
    .bind(EventKind::GpsPing.as_str())
    .bind(ping.driver_id)
    .bind(&ping.device_id)
    .bind(ping.lat)
    .bind(ping.lng)
    .bind(ping.accuracy_m)
    .bind(ping.recorded_at)
    .execute(pool)
    .await?
    .rows_affected();
    Ok(inserted == 1)
}

async fn update_last_position(pool: &PgPool, ping: &PingInput) -> AppResult<()> {
    sqlx::query(
        "UPDATE tracked_shipments \
         SET last_lat = $2, last_lng = $3, last_seen_at = $4, updated_at = now() \
         WHERE driver_id = $1 AND status <> $5 \
           AND (last_seen_at IS NULL OR last_seen_at < $4)",
    )
    .bind(ping.driver_id)
    .bind(ping.lat)
    .bind(ping.lng)
    .bind(ping.recorded_at)
    .bind(ShipmentStatus::Delivered.as_str())
    .execute(pool)
    .await?;
    Ok(())
}

/// Marks every in-transit shipment of this driver whose planned stop the
/// ping falls inside as ARRIVED. Returns the shipment ids that arrived.
async fn check_arrivals(pool: &PgPool, ping: &PingInput) -> AppResult<Vec<Uuid>> {
    let open_stops = sqlx::query_as::<_, (Uuid, f64, f64)>(
        "SELECT s.shipment_id, p.lat, p.lng \
         FROM tracked_shipments s \
         JOIN LATERAL ( \
             SELECT lat, lng FROM planned_stops ps \
             WHERE ps.shipment_id = s.shipment_id \
             ORDER BY ps.planned_at DESC LIMIT 1 \
         ) p ON true \
         WHERE s.driver_id = $1 AND s.status = $2",
    )
    .bind(ping.driver_id)
    .bind(ShipmentStatus::InTransit.as_str())
    .fetch_all(pool)
    .await?;

    let position = Coordinate::new(ping.lat, ping.lng);
    let mut arrived = Vec::new();

    for (shipment_id, lat, lng) in open_stops {
        let Some(arrival) = detect_arrival(position, ping.accuracy_m, Coordinate::new(lat, lng))
        else {
            continue;
        };

        let updated = sqlx::query(
            "UPDATE tracked_shipments SET status = $2, arrived_at = $3, updated_at = now() \
             WHERE shipment_id = $1 AND status = $4",
        )
        .bind(shipment_id)
        .bind(ShipmentStatus::Arrived.as_str())
        .bind(ping.recorded_at)
        .bind(ShipmentStatus::InTransit.as_str())
        .execute(pool)
        .await?
        .rows_affected();

        if updated == 1 {
            tracing::info!(
                %shipment_id,
                driver_id = %ping.driver_id,
                distance_m = arrival.distance_m,
                "geofence arrival"
            );
            arrived.push(shipment_id);
        }
    }

    Ok(arrived)
}
