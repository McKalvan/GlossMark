use std::fmt;
use std::str::FromStr;

use chrono::{DateTime, NaiveDate, Utc};
use serde::{Deserialize, Serialize};
use uuid::Uuid;

/// CloudEvents-style envelope shared by every Kestrel Freight topic.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Envelope<T> {
    #[serde(rename = "type")]
    pub event_type: String,
    pub id: Uuid,
    pub time: DateTime<Utc>,
    pub data: T,
}

impl<T> Envelope<T> {
    pub fn new(event_type: &str, data: T) -> Self {
        Self {
            event_type: event_type.to_owned(),
            id: Uuid::new_v4(),
            time: Utc::now(),
            data,
        }
    }
}

// @gloss represents frt:ShipmentPickedUp conf=0.95
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ShipmentPickedUp {
    pub shipment_id: Uuid,
    pub tracking_number: String,
    pub driver_id: Uuid,
    pub route_id: Uuid,
    pub picked_up_at: DateTime<Utc>,
    pub lat: f64,
    pub lng: f64,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RoutePlanned {
    pub route_id: Uuid,
    pub driver_id: Uuid,
    pub date: NaiveDate,
    pub stops: Vec<RouteStop>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RouteStop {
    pub shipment_id: Uuid,
    pub seq: u32,
    pub lat: f64,
    pub lng: f64,
    pub eta_from: DateTime<Utc>,
    pub eta_to: DateTime<Utc>,
}

// @gloss represents frt:ShipmentDelivered conf=0.95
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ShipmentDelivered {
    pub shipment_id: Uuid,
    pub tracking_number: String,
    pub delivered_at: DateTime<Utc>,
    pub lat: f64,
    pub lng: f64,
    pub proof: Proof,
    pub within_window: bool,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Proof {
    pub kind: ProofKind,
    #[serde(rename = "ref")]
    pub reference: String,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum ProofKind {
    Signature,
    Photo,
    Geofence,
}

impl ProofKind {
    pub fn as_str(self) -> &'static str {
        match self {
            ProofKind::Signature => "SIGNATURE",
            ProofKind::Photo => "PHOTO",
            ProofKind::Geofence => "GEOFENCE",
        }
    }
}

impl fmt::Display for ProofKind {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.as_str())
    }
}

#[derive(Debug, thiserror::Error)]
#[error("unknown proof kind {0:?}")]
pub struct UnknownProofKind(pub String);

impl FromStr for ProofKind {
    type Err = UnknownProofKind;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s {
            "SIGNATURE" => Ok(ProofKind::Signature),
            "PHOTO" => Ok(ProofKind::Photo),
            "GEOFENCE" => Ok(ProofKind::Geofence),
            other => Err(UnknownProofKind(other.to_owned())),
        }
    }
}
