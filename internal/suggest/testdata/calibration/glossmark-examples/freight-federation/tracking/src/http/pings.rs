use axum::extract::State;
use axum::http::StatusCode;
use axum::Json;
use serde::Deserialize;

use crate::error::{AppError, AppResult};
use crate::http::AppState;
use crate::ingest::ping_ingestor::{ingest_batch, PingBatchResult, PingInput};

/// Driver apps flush their buffered pings in batches of at most this size.
pub const MAX_BATCH_SIZE: usize = 500;

#[derive(Debug, Deserialize)]
pub struct PingBatch {
    pub pings: Vec<PingInput>,
}

/// POST /pings
pub async fn post_pings(
    State(state): State<AppState>,
    Json(batch): Json<PingBatch>,
) -> AppResult<(StatusCode, Json<PingBatchResult>)> {
    if batch.pings.is_empty() {
        return Err(AppError::Validation("batch contains no pings".to_owned()));
    }
    if batch.pings.len() > MAX_BATCH_SIZE {
        return Err(AppError::Validation(format!(
            "batch of {} pings exceeds the limit of {MAX_BATCH_SIZE}",
            batch.pings.len()
        )));
    }

    let result = ingest_batch(&state.pool, batch.pings).await?;
    Ok((StatusCode::ACCEPTED, Json(result)))
}
