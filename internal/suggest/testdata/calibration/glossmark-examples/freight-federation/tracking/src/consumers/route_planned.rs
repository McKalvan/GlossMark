//! `dispatch.routes.v1`: RoutePlanned carries the driver's ordered stops and
//! their ETA windows. A re-plan of the same route replaces its stops.

use std::time::Duration;

use rdkafka::consumer::{CommitMode, Consumer, StreamConsumer};
use rdkafka::Message;
use sqlx::PgPool;

use crate::error::AppResult;
use crate::events::types::{Envelope, RoutePlanned};
use crate::events::ROUTE_PLANNED;

pub async fn run(consumer: StreamConsumer, pool: PgPool) {
    loop {
        let message = match consumer.recv().await {
            Ok(message) => message,
            Err(err) => {
                tracing::error!(error = %err, "dispatch.routes.v1 receive failed");
                tokio::time::sleep(Duration::from_secs(1)).await;
                continue;
            }
        };

        if let Err(err) = handle(&pool, message.payload()).await {
            tracing::warn!(
                error = %err,
                partition = message.partition(),
                offset = message.offset(),
                "skipping RoutePlanned message"
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
    let envelope: Envelope<RoutePlanned> = serde_json::from_slice(bytes)?;
    if envelope.event_type != ROUTE_PLANNED {
        tracing::debug!(event_type = %envelope.event_type, "ignoring unexpected event type");
        return Ok(());
    }
    store_planned_stops(pool, &envelope.data).await
}

// @gloss implements .PlannedStopSync conf=0.55
pub async fn store_planned_stops(pool: &PgPool, route: &RoutePlanned) -> AppResult<()> {
    let mut tx = pool.begin().await?;

    sqlx::query("DELETE FROM planned_stops WHERE route_id = $1")
        .bind(route.route_id)
        .execute(&mut *tx)
        .await?;

    for stop in &route.stops {
        sqlx::query(
            "INSERT INTO planned_stops \
                (route_id, shipment_id, seq, driver_id, route_date, lat, lng, eta_from, eta_to) \
             VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)",
        )
        .bind(route.route_id)
        .bind(stop.shipment_id)
        .bind(stop.seq as i32)
        .bind(route.driver_id)
        .bind(route.date)
        .bind(stop.lat)
        .bind(stop.lng)
        .bind(stop.eta_from)
        .bind(stop.eta_to)
        .execute(&mut *tx)
        .await?;
    }

    tx.commit().await?;
    tracing::info!(
        route_id = %route.route_id,
        driver_id = %route.driver_id,
        stops = route.stops.len(),
        "planned stops stored"
    );
    Ok(())
}
