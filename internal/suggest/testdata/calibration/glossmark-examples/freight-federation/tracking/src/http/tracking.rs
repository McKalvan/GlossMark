//! Customer-facing tracking. Customers see scan history and the latest
//! position, never the driver's raw GPS trail.

use axum::extract::{Path, State};
use axum::Json;
use chrono::{DateTime, Utc};
use serde::Serialize;
use uuid::Uuid;

use crate::delivery::window::window_end;
use crate::error::{AppError, AppResult};
use crate::http::AppState;
use crate::ingest::scan_ingestor::parse_tracking_number;
use crate::models::{Delivery, EventKind, PlannedStop, TrackedShipment, TrackingEvent};

const MAX_EVENTS: i64 = 50;

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TrackingView {
    pub tracking_number: String,
    pub shipment_id: Uuid,
    pub status: String,
    pub picked_up_at: DateTime<Utc>,
    pub last_position: Option<Position>,
    pub eta_window: Option<EtaWindow>,
    pub delivery: Option<DeliveryView>,
    pub events: Vec<EventView>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Position {
    pub lat: f64,
    pub lng: f64,
    pub seen_at: DateTime<Utc>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct EtaWindow {
    pub from: DateTime<Utc>,
    pub to: DateTime<Utc>,
    pub promised_by: DateTime<Utc>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DeliveryView {
    pub delivered_at: DateTime<Utc>,
    pub proof_kind: String,
    pub within_window: bool,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct EventView {
    pub scan_type: Option<String>,
    pub location: Option<String>,
    pub recorded_at: DateTime<Utc>,
}

impl From<TrackingEvent> for EventView {
    fn from(event: TrackingEvent) -> Self {
        Self {
            scan_type: event.scan_type,
            location: event.scan_location,
            recorded_at: event.recorded_at,
        }
    }
}

/// GET /tracking/:tracking_number
// @gloss implements .TrackingQuery conf=0.85
pub async fn get_tracking(
    State(state): State<AppState>,
    Path(tracking_number): Path<String>,
) -> AppResult<Json<TrackingView>> {
    let tracking_number = parse_tracking_number(&tracking_number)?;
    let pool = &state.pool;

    let shipment = sqlx::query_as::<_, TrackedShipment>(
        "SELECT * FROM tracked_shipments WHERE tracking_number = $1",
    )
    .bind(tracking_number.as_str())
    .fetch_optional(pool)
    .await?
    .ok_or_else(|| AppError::NotFound(format!("shipment {tracking_number}")))?;

    let stop = sqlx::query_as::<_, PlannedStop>(
        "SELECT * FROM planned_stops WHERE shipment_id = $1 ORDER BY planned_at DESC LIMIT 1",
    )
    .bind(shipment.shipment_id)
    .fetch_optional(pool)
    .await?;

    let delivery = sqlx::query_as::<_, Delivery>("SELECT * FROM deliveries WHERE shipment_id = $1")
        .bind(shipment.shipment_id)
        .fetch_optional(pool)
        .await?;

    let events = sqlx::query_as::<_, TrackingEvent>(
        "SELECT * FROM tracking_events \
         WHERE shipment_id = $1 AND kind = $2 \
         ORDER BY recorded_at DESC \
         LIMIT $3",
    )
    .bind(shipment.shipment_id)
    .bind(EventKind::Scan.as_str())
    .bind(MAX_EVENTS)
    .fetch_all(pool)
    .await?;

    let last_position = match (shipment.last_lat, shipment.last_lng, shipment.last_seen_at) {
        (Some(lat), Some(lng), Some(seen_at)) if !shipment.is_delivered() => {
            Some(Position { lat, lng, seen_at })
        }
        _ => None,
    };

    Ok(Json(TrackingView {
        tracking_number: shipment.tracking_number,
        shipment_id: shipment.shipment_id,
        status: shipment.status,
        picked_up_at: shipment.picked_up_at,
        last_position,
        eta_window: stop.map(|s| EtaWindow {
            from: s.eta_from,
            to: s.eta_to,
            promised_by: window_end(s.eta_from),
        }),
        delivery: delivery.map(|d| DeliveryView {
            delivered_at: d.delivered_at,
            proof_kind: d.proof_kind,
            within_window: d.within_window,
        }),
        events: events.into_iter().map(EventView::from).collect(),
    }))
}
