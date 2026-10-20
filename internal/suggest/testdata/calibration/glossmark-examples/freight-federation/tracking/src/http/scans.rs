use axum::extract::State;
use axum::http::StatusCode;
use axum::Json;
use serde::Serialize;
use uuid::Uuid;

use crate::error::AppResult;
use crate::http::AppState;
use crate::ingest::scan_ingestor::{ingest_scan, ScanInput};

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ScanAccepted {
    pub event_id: i64,
    pub tracking_number: String,
    pub shipment_id: Option<Uuid>,
    pub delivered: bool,
}

/// POST /scans
pub async fn post_scan(
    State(state): State<AppState>,
    Json(scan): Json<ScanInput>,
) -> AppResult<(StatusCode, Json<ScanAccepted>)> {
    let outcome = ingest_scan(&state.pool, &state.publisher, scan).await?;
    Ok((
        StatusCode::CREATED,
        Json(ScanAccepted {
            event_id: outcome.event_id,
            tracking_number: outcome.tracking_number.to_string(),
            shipment_id: outcome.shipment_id,
            delivered: outcome.delivered,
        }),
    ))
}
