pub mod picked_up;
pub mod route_planned;

use rdkafka::consumer::{Consumer, StreamConsumer};
use rdkafka::error::KafkaError;
use rdkafka::ClientConfig;

/// Builds a manually-committing consumer subscribed to a single topic.
pub fn build_consumer(brokers: &str, group_id: &str, topic: &str) -> Result<StreamConsumer, KafkaError> {
    let consumer: StreamConsumer = ClientConfig::new()
        .set("bootstrap.servers", brokers)
        .set("group.id", format!("{group_id}-{topic}"))
        .set("client.id", format!("{group_id}-{topic}"))
        .set("enable.auto.commit", "false")
        .set("auto.offset.reset", "earliest")
        .set("session.timeout.ms", "10000")
        .create()?;
    consumer.subscribe(&[topic])?;
    Ok(consumer)
}
