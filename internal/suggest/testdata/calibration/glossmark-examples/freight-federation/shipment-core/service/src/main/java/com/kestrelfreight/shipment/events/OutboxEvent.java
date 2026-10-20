package com.kestrelfreight.shipment.events;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;

import java.time.Instant;
import java.util.Optional;
import java.util.UUID;

// @gloss represents .Outbox conf=0.64
@Entity
@Table(name = "outbox_events")
public class OutboxEvent {

    @Id
    private UUID id;

    @Column(name = "aggregate_type", nullable = false, length = 64)
    private String aggregateType;

    @Column(name = "aggregate_id", nullable = false)
    private UUID aggregateId;

    @Column(name = "event_type", nullable = false, length = 64)
    private String eventType;

    @Column(name = "topic", nullable = false, length = 128)
    private String topic;

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(name = "payload", nullable = false, columnDefinition = "jsonb")
    private String payload;

    @Column(name = "created_at", nullable = false)
    private Instant createdAt;

    @Column(name = "sent_at")
    private Instant sentAt;

    @Column(name = "attempts", nullable = false)
    private int attempts;

    @Column(name = "last_error", length = 1024)
    private String lastError;

    protected OutboxEvent() {
    }

    private OutboxEvent(String aggregateType, UUID aggregateId, String eventType, String topic, String payload,
                        Instant createdAt) {
        this.id = UUID.randomUUID();
        this.aggregateType = aggregateType;
        this.aggregateId = aggregateId;
        this.eventType = eventType;
        this.topic = topic;
        this.payload = payload;
        this.createdAt = createdAt;
    }

    public static OutboxEvent shipmentBooked(UUID shipmentId, String payload, Instant createdAt) {
        return new OutboxEvent("Shipment", shipmentId, ShipmentBookedEvent.TYPE, ShipmentBookedEvent.TOPIC,
                payload, createdAt);
    }

    public void markSent(Instant at) {
        this.sentAt = at;
        this.attempts++;
        this.lastError = null;
    }

    public void markFailed(String error) {
        this.attempts++;
        this.lastError = error == null ? null : error.substring(0, Math.min(error.length(), 1024));
    }

    public UUID getId() {
        return id;
    }

    public String getAggregateType() {
        return aggregateType;
    }

    public UUID getAggregateId() {
        return aggregateId;
    }

    public String getEventType() {
        return eventType;
    }

    public String getTopic() {
        return topic;
    }

    public String getPayload() {
        return payload;
    }

    public Instant getCreatedAt() {
        return createdAt;
    }

    public Optional<Instant> getSentAt() {
        return Optional.ofNullable(sentAt);
    }

    public int getAttempts() {
        return attempts;
    }

    public Optional<String> getLastError() {
        return Optional.ofNullable(lastError);
    }
}
