//! Proof of delivery. A delivery is confirmed by a signature, a photo, or a
//! geofence arrival; the confirmation decides whether it was inside the
//! delivery window and announces it as ShipmentDelivered.

use chrono::{DateTime, Utc};
use sqlx::PgPool;
use uuid::Uuid;

use crate::delivery::geofence::{haversine_m, Coordinate, ARRIVAL_RADIUS_M};
use crate::delivery::window::within_window;
use crate::error::{AppError, AppResult};
use crate::events::publisher::EventPublisher;
use crate::events::types::{Proof, ProofKind, ShipmentDelivered};
use crate::models::{Delivery, PlannedStop, ShipmentStatus, TrackedShipment};

/// What tracking knows about a shipment at the moment it is delivered.
#[derive(Debug, Clone)]
pub struct DeliveryContext {
    pub shipment_id: Uuid,
    pub tracking_number: String,
    pub promised_start: DateTime<Utc>,
    pub destination: Coordinate,
}

/// What the driver's device reports when handing over the shipment.
#[derive(Debug, Clone)]
pub struct ProofOfDelivery {
    pub delivered_at: DateTime<Utc>,
    pub position: Coordinate,
    pub proof: Proof,
}

#[derive(Debug, Clone, PartialEq, thiserror::Error)]
pub enum ConfirmationError {
    #[error("proof of delivery reference is empty")]
    EmptyProofReference,
    #[error("delivery position is not a valid coordinate")]
    InvalidPosition,
    #[error("geofence proof needs the driver within {radius_m} m of the delivery address, was {distance_m:.0} m away")]
    OutsideGeofence { distance_m: f64, radius_m: f64 },
}

/// Pure decision: validates the proof and builds the ShipmentDelivered event,
/// including whether the delivery landed inside the delivery window.
pub fn decide_delivery(
    ctx: &DeliveryContext,
    pod: &ProofOfDelivery,
) -> Result<ShipmentDelivered, ConfirmationError> {
    if pod.proof.reference.trim().is_empty() {
        return Err(ConfirmationError::EmptyProofReference);
    }
    if !pod.position.is_valid() {
        return Err(ConfirmationError::InvalidPosition);
    }
    if pod.proof.kind == ProofKind::Geofence {
        let distance_m = haversine_m(pod.position, ctx.destination);
        if distance_m > ARRIVAL_RADIUS_M {
            return Err(ConfirmationError::OutsideGeofence {
                distance_m,
                radius_m: ARRIVAL_RADIUS_M,
            });
        }
    }

    Ok(ShipmentDelivered {
        shipment_id: ctx.shipment_id,
        tracking_number: ctx.tracking_number.clone(),
        delivered_at: pod.delivered_at,
        lat: pod.position.lat,
        lng: pod.position.lng,
        proof: pod.proof.clone(),
        within_window: within_window(ctx.promised_start, pod.delivered_at),
    })
}

/// Records proof of delivery, marks the shipment delivered and publishes
/// ShipmentDelivered. The database write commits before the publish; a
/// failed publish leaves `published_at` NULL for [`republish_pending`].
// @gloss implements .DeliveryConfirmation conf=0.9
pub async fn record_proof_of_delivery(
    pool: &PgPool,
    publisher: &EventPublisher,
    shipment_id: Uuid,
    pod: ProofOfDelivery,
) -> AppResult<ShipmentDelivered> {
    let mut tx = pool.begin().await?;

    let shipment = sqlx::query_as::<_, TrackedShipment>(
        "SELECT * FROM tracked_shipments WHERE shipment_id = $1 FOR UPDATE",
    )
    .bind(shipment_id)
    .fetch_optional(&mut *tx)
    .await?
    .ok_or_else(|| AppError::NotFound(format!("shipment {shipment_id}")))?;

    if shipment.is_delivered() {
        return Err(AppError::Conflict(format!(
            "shipment {} is already delivered",
            shipment.tracking_number
        )));
    }

    let stop = sqlx::query_as::<_, PlannedStop>(
        "SELECT * FROM planned_stops WHERE shipment_id = $1 ORDER BY planned_at DESC LIMIT 1",
    )
    .bind(shipment_id)
    .fetch_optional(&mut *tx)
    .await?
    .ok_or_else(|| AppError::NotFound(format!("planned stop for shipment {shipment_id}")))?;

    let ctx = DeliveryContext {
        shipment_id,
        tracking_number: shipment.tracking_number.clone(),
        promised_start: stop.eta_from,
        destination: Coordinate::new(stop.lat, stop.lng),
    };
    let event = decide_delivery(&ctx, &pod)?;
    let delivery_id = Uuid::new_v4();

    sqlx::query(
        "INSERT INTO deliveries \
            (id, shipment_id, tracking_number, delivered_at, lat, lng, proof_kind, proof_ref, promised_start, within_window) \
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)",
    )
    .bind(delivery_id)
    .bind(shipment_id)
    .bind(&event.tracking_number)
    .bind(event.delivered_at)
    .bind(event.lat)
    .bind(event.lng)
    .bind(event.proof.kind.as_str())
    .bind(&event.proof.reference)
    .bind(ctx.promised_start)
    .bind(event.within_window)
    .execute(&mut *tx)
    .await?;

    sqlx::query(
        "UPDATE tracked_shipments \
         SET status = $2, last_lat = $3, last_lng = $4, last_seen_at = $5, updated_at = now() \
         WHERE shipment_id = $1",
    )
    .bind(shipment_id)
    .bind(ShipmentStatus::Delivered.as_str())
    .bind(event.lat)
    .bind(event.lng)
    .bind(event.delivered_at)
    .execute(&mut *tx)
    .await?;

    tx.commit().await?;

    match publisher.publish_shipment_delivered(&event).await {
        Ok(()) => mark_published(pool, delivery_id).await?,
        Err(err) => tracing::warn!(
            %shipment_id,
            error = %err,
            "ShipmentDelivered not published; republish sweep will retry"
        ),
    }

    Ok(event)
}

/// Re-publishes deliveries whose ShipmentDelivered never made it to Kafka.
pub async fn republish_pending(
    pool: &PgPool,
    publisher: &EventPublisher,
    limit: i64,
) -> AppResult<usize> {
    let pending = sqlx::query_as::<_, Delivery>(
        "SELECT * FROM deliveries \
         WHERE published_at IS NULL AND created_at < now() - interval '30 seconds' \
         ORDER BY created_at \
         LIMIT $1",
    )
    .bind(limit)
    .fetch_all(pool)
    .await?;

    for delivery in &pending {
        let event = delivery_event(delivery)?;
        publisher.publish_shipment_delivered(&event).await?;
        mark_published(pool, delivery.id).await?;
    }
    Ok(pending.len())
}

pub fn delivery_event(delivery: &Delivery) -> AppResult<ShipmentDelivered> {
    let kind: ProofKind = delivery
        .proof_kind
        .parse()
        .map_err(|err: crate::events::types::UnknownProofKind| AppError::Validation(err.to_string()))?;

    Ok(ShipmentDelivered {
        shipment_id: delivery.shipment_id,
        tracking_number: delivery.tracking_number.clone(),
        delivered_at: delivery.delivered_at,
        lat: delivery.lat,
        lng: delivery.lng,
        proof: Proof {
            kind,
            reference: delivery.proof_ref.clone(),
        },
        within_window: delivery.within_window,
    })
}

async fn mark_published(pool: &PgPool, delivery_id: Uuid) -> AppResult<()> {
    sqlx::query("UPDATE deliveries SET published_at = now() WHERE id = $1")
        .bind(delivery_id)
        .execute(pool)
        .await?;
    Ok(())
}
