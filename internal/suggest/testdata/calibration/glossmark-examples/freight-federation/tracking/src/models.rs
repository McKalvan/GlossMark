use chrono::{DateTime, NaiveDate, Utc};
use serde::Serialize;
use sqlx::FromRow;
use uuid::Uuid;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ShipmentStatus {
    InTransit,
    Arrived,
    Delivered,
}

impl ShipmentStatus {
    pub fn as_str(self) -> &'static str {
        match self {
            ShipmentStatus::InTransit => "IN_TRANSIT",
            ShipmentStatus::Arrived => "ARRIVED",
            ShipmentStatus::Delivered => "DELIVERED",
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EventKind {
    GpsPing,
    Scan,
}

impl EventKind {
    pub fn as_str(self) -> &'static str {
        match self {
            EventKind::GpsPing => "GPS_PING",
            EventKind::Scan => "SCAN",
        }
    }
}

// @gloss represents frt:Shipment conf=0.45
#[derive(Debug, Clone, FromRow, Serialize)]
pub struct TrackedShipment {
    pub shipment_id: Uuid,
    pub tracking_number: String,
    pub driver_id: Uuid,
    pub route_id: Uuid,
    pub status: String,
    pub picked_up_at: DateTime<Utc>,
    pub pickup_lat: f64,
    pub pickup_lng: f64,
    pub last_lat: Option<f64>,
    pub last_lng: Option<f64>,
    pub last_seen_at: Option<DateTime<Utc>>,
    pub arrived_at: Option<DateTime<Utc>>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

impl TrackedShipment {
    pub fn is_delivered(&self) -> bool {
        self.status == ShipmentStatus::Delivered.as_str()
    }
}

#[derive(Debug, Clone, FromRow, Serialize)]
pub struct TrackingEvent {
    pub id: i64,
    pub kind: String,
    pub shipment_id: Option<Uuid>,
    pub tracking_number: Option<String>,
    pub driver_id: Option<Uuid>,
    pub device_id: String,
    pub lat: Option<f64>,
    pub lng: Option<f64>,
    pub accuracy_m: Option<f32>,
    pub scan_type: Option<String>,
    pub scan_location: Option<String>,
    pub raw_barcode: Option<String>,
    pub recorded_at: DateTime<Utc>,
    pub received_at: DateTime<Utc>,
}

#[derive(Debug, Clone, FromRow, Serialize)]
pub struct PlannedStop {
    pub route_id: Uuid,
    pub shipment_id: Uuid,
    pub seq: i32,
    pub driver_id: Uuid,
    pub route_date: NaiveDate,
    pub lat: f64,
    pub lng: f64,
    pub eta_from: DateTime<Utc>,
    pub eta_to: DateTime<Utc>,
    pub planned_at: DateTime<Utc>,
}

#[derive(Debug, Clone, FromRow, Serialize)]
pub struct Delivery {
    pub id: Uuid,
    pub shipment_id: Uuid,
    pub tracking_number: String,
    pub delivered_at: DateTime<Utc>,
    pub lat: f64,
    pub lng: f64,
    pub proof_kind: String,
    pub proof_ref: String,
    pub promised_start: DateTime<Utc>,
    pub within_window: bool,
    pub published_at: Option<DateTime<Utc>>,
    pub created_at: DateTime<Utc>,
}
