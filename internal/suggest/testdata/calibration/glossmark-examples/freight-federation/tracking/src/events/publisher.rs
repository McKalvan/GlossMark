use std::time::Duration;

use rdkafka::error::KafkaError;
use rdkafka::producer::{FutureProducer, FutureRecord};
use rdkafka::ClientConfig;

use crate::error::{AppError, AppResult};
use crate::events::types::{Envelope, ShipmentDelivered};
use crate::events::{SHIPMENT_DELIVERED, TOPIC_SHIPMENT_DELIVERED};

const QUEUE_TIMEOUT: Duration = Duration::from_secs(5);

#[derive(Clone)]
pub struct EventPublisher {
    producer: FutureProducer,
}

impl EventPublisher {
    pub fn new(brokers: &str) -> Result<Self, KafkaError> {
        let producer: FutureProducer = ClientConfig::new()
            .set("bootstrap.servers", brokers)
            .set("client.id", "tracking")
            .set("enable.idempotence", "true")
            .set("acks", "all")
            .set("compression.type", "lz4")
            .set("message.timeout.ms", "15000")
            .create()?;
        Ok(Self { producer })
    }

    /// Publishes to `shipment.delivered.v1`, keyed by shipment id so every
    /// event for one shipment lands on the same partition.
    // @gloss publishes frt:ShipmentDelivered conf=0.9
    pub async fn publish_shipment_delivered(&self, event: &ShipmentDelivered) -> AppResult<()> {
        let envelope = Envelope::new(SHIPMENT_DELIVERED, event);
        let payload = serde_json::to_vec(&envelope)?;
        let key = event.shipment_id.to_string();

        let record = FutureRecord::to(TOPIC_SHIPMENT_DELIVERED)
            .key(&key)
            .payload(&payload);

        let (partition, offset) = self
            .producer
            .send(record, QUEUE_TIMEOUT)
            .await
            .map_err(|(err, _)| AppError::Kafka(err))?;

        tracing::info!(
            shipment_id = %event.shipment_id,
            tracking_number = %event.tracking_number,
            within_window = event.within_window,
            partition,
            offset,
            "published ShipmentDelivered"
        );
        Ok(())
    }
}
