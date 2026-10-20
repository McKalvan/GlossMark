use std::time::Duration;

use kestrel_tracking::config::Config;
use kestrel_tracking::consumers::{self, picked_up, route_planned};
use kestrel_tracking::delivery::confirmation::republish_pending;
use kestrel_tracking::events::publisher::EventPublisher;
use kestrel_tracking::events::{TOPIC_DISPATCH_ROUTES, TOPIC_SHIPMENT_PICKED_UP};
use kestrel_tracking::http::{self, AppState};
use sqlx::postgres::PgPoolOptions;
use tokio::net::TcpListener;
use tracing_subscriber::EnvFilter;

const REPUBLISH_INTERVAL: Duration = Duration::from_secs(60);
const REPUBLISH_BATCH: i64 = 100;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::from_default_env())
        .json()
        .init();

    let config = Config::from_env()?;

    let pool = PgPoolOptions::new()
        .max_connections(config.database_max_connections)
        .acquire_timeout(Duration::from_secs(5))
        .connect(&config.database_url)
        .await?;
    sqlx::migrate!("./migrations").run(&pool).await?;

    let publisher = EventPublisher::new(&config.kafka_brokers)?;

    let picked_up_consumer = consumers::build_consumer(
        &config.kafka_brokers,
        &config.kafka_group_id,
        TOPIC_SHIPMENT_PICKED_UP,
    )?;
    let routes_consumer = consumers::build_consumer(
        &config.kafka_brokers,
        &config.kafka_group_id,
        TOPIC_DISPATCH_ROUTES,
    )?;
    tokio::spawn(picked_up::run(picked_up_consumer, pool.clone()));
    tokio::spawn(route_planned::run(routes_consumer, pool.clone()));

    {
        let pool = pool.clone();
        let publisher = publisher.clone();
        tokio::spawn(async move {
            let mut ticker = tokio::time::interval(REPUBLISH_INTERVAL);
            loop {
                ticker.tick().await;
                match republish_pending(&pool, &publisher, REPUBLISH_BATCH).await {
                    Ok(0) => {}
                    Ok(n) => tracing::info!(republished = n, "republished pending ShipmentDelivered events"),
                    Err(err) => tracing::warn!(error = %err, "republish sweep failed"),
                }
            }
        });
    }

    let app = http::router(AppState { pool, publisher });
    let listener = TcpListener::bind(config.http_addr).await?;
    tracing::info!(addr = %config.http_addr, "tracking service listening");

    axum::serve(listener, app)
        .with_graceful_shutdown(shutdown_signal())
        .await?;
    Ok(())
}

async fn shutdown_signal() {
    let ctrl_c = async {
        let _ = tokio::signal::ctrl_c().await;
    };

    #[cfg(unix)]
    let terminate = async {
        if let Ok(mut sig) = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate()) {
            sig.recv().await;
        }
    };
    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => {},
        _ = terminate => {},
    }
    tracing::info!("shutdown signal received");
}
