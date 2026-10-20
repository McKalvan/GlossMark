use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;
use serde_json::json;

use crate::delivery::confirmation::ConfirmationError;
use crate::ingest::scan_ingestor::ScanParseError;

pub type AppResult<T> = Result<T, AppError>;

#[derive(Debug, thiserror::Error)]
pub enum AppError {
    #[error("database error: {0}")]
    Database(#[from] sqlx::Error),
    #[error("kafka error: {0}")]
    Kafka(#[from] rdkafka::error::KafkaError),
    #[error("malformed payload: {0}")]
    Json(#[from] serde_json::Error),
    #[error(transparent)]
    Scan(#[from] ScanParseError),
    #[error(transparent)]
    Confirmation(#[from] ConfirmationError),
    #[error("validation failed: {0}")]
    Validation(String),
    #[error("{0} not found")]
    NotFound(String),
    #[error("conflict: {0}")]
    Conflict(String),
}

impl AppError {
    fn status_and_code(&self) -> (StatusCode, &'static str) {
        match self {
            AppError::Database(_) | AppError::Kafka(_) => {
                (StatusCode::INTERNAL_SERVER_ERROR, "internal_error")
            }
            AppError::Json(_) => (StatusCode::BAD_REQUEST, "malformed_payload"),
            AppError::Scan(_) => (StatusCode::UNPROCESSABLE_ENTITY, "invalid_barcode"),
            AppError::Confirmation(_) => (StatusCode::UNPROCESSABLE_ENTITY, "confirmation_rejected"),
            AppError::Validation(_) => (StatusCode::UNPROCESSABLE_ENTITY, "validation_failed"),
            AppError::NotFound(_) => (StatusCode::NOT_FOUND, "not_found"),
            AppError::Conflict(_) => (StatusCode::CONFLICT, "conflict"),
        }
    }
}

impl IntoResponse for AppError {
    fn into_response(self) -> Response {
        let (status, code) = self.status_and_code();
        let message = if status.is_server_error() {
            tracing::error!(error = %self, "request failed");
            "internal error".to_owned()
        } else {
            self.to_string()
        };
        (status, Json(json!({ "error": code, "message": message }))).into_response()
    }
}
