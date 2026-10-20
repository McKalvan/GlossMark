//! `shipment.picked-up.v1`: a driver has collected the shipment, so tracking
//! starts following it.

use std::time::Duration;

use rdkafka::consumer::{CommitMode, Consumer, StreamConsumer};
use rdkafka::Message;
use sqlx::PgPool;

use crate::error::AppResult;
use crate::events::types::{Envelope, ShipmentPickedUp};
use crate::events::SHIPMENT_PICKED_UP;
use crate::models::ShipmentStatus;

// @gloss subscribes frt:ShipmentPickedUp conf=0.9
pub async fn run(consumer: StreamConsumer, pool: PgPool) {
    loop {
        let message = match consumer.recv().await {
            Ok(message) => message,
            Err(err) => {
                tracing::error!(error = %err, "shipment.picked-up.v1 receive failed");
                tokio::time::sleep(Duration::from_secs(1)).await;
                continue;
            }
        };

        if let Err(err) = handle(&pool, message.payload()).await {
            tracing::warn!(
                error = %err,
                partition = message.partition(),
                offset = message.offset(),
                "skipping ShipmentPickedUp message"
            );
        }
        if let Err(err) = consumer.commit_message(&message, CommitMode::Async) {
            tracing::warn!(error = %err, "offset commit failed");
        }
    }
}

pub async fn handle(pool: &PgPool, payload: Option<&[u8]>) -> AppResult<()> {
    let Some(bytes) = payload else {
        return Ok(());
    };
    let envelope: Envelope<ShipmentPickedUp> = serde_json::from_slice(bytes)?;
    if envelope.event_type != SHIPMENT_PICKED_UP {
        tracing::debug!(event_type = %envelope.event_type, "ignoring unexpected event type");
        return Ok(());
    }
    start_tracking(pool, &envelope.data).await
}

/// Idempotent: redelivered events leave the existing row untouched.
// @gloss implements .PickedUpIntake conf=0.85
pub async fn start_tracking(pool: &PgPool, event: &ShipmentPickedUp) -> AppResult<()> {
    let inserted = sqlx::query(
        "INSERT INTO tracked_shipments \
            (shipment_id, tracking_number, driver_id, route_id, status, picked_up_at, pickup_lat, pickup_lng, last_lat, last_lng, last_seen_at) \
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $7, $8, $6) \
         ON CONFLICT (shipment_id) DO NOTHING",
    )
    .bind(event.shipment_id)
    .bind(&event.tracking_number)
    .bind(event.driver_id)
    .bind(event.route_id)
    .bind(ShipmentStatus::InTransit.as_str())
    .bind(event.picked_up_at)
    .bind(event.lat)
    .bind(event.lng)
    .execute(pool)
    .await?
    .rows_affected();

    if inserted == 1 {
        tracing::info!(
            shipment_id = %event.shipment_id,
            tracking_number = %event.tracking_number,
            driver_id = %event.driver_id,
            "tracking started"
        );
    }
    Ok(())
}
