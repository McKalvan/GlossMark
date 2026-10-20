pub mod pings;
pub mod scans;
pub mod tracking;

use axum::routing::{get, post};
use axum::Router;
use sqlx::PgPool;

use crate::events::publisher::EventPublisher;

#[derive(Clone)]
pub struct AppState {
    pub pool: PgPool,
    pub publisher: EventPublisher,
}

pub fn router(state: AppState) -> Router {
    Router::new()
        .route("/pings", post(pings::post_pings))
        .route("/scans", post(scans::post_scan))
        .route("/tracking/:tracking_number", get(tracking::get_tracking))
        .route("/healthz", get(|| async { "ok" }))
        .with_state(state)
}
