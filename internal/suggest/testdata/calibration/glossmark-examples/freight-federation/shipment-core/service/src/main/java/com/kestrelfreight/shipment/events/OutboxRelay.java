package com.kestrelfreight.shipment.events;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.data.domain.Limit;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;
import org.springframework.transaction.annotation.Transactional;

import java.time.Clock;
import java.time.Duration;
import java.util.List;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;

/**
 * Publishes rows written to the transactional outbox. Each message is a CloudEvents-style envelope
 * ({@code type}, {@code id}, {@code time}, {@code data}) keyed by the aggregate id so that all events for one
 * shipment land on the same partition.
 */
// @gloss implements .ShipmentBookedEmitter conf=0.9
@Component
public class OutboxRelay {

    private static final Logger log = LoggerFactory.getLogger(OutboxRelay.class);
    private static final Duration SEND_TIMEOUT = Duration.ofSeconds(10);

    private final OutboxRepository outbox;
    private final KafkaTemplate<String, String> kafka;
    private final ObjectMapper objectMapper;
    private final Clock clock;
    private final int batchSize;

    public OutboxRelay(OutboxRepository outbox, KafkaTemplate<String, String> kafka, ObjectMapper objectMapper,
                       Clock clock, @Value("${kestrel.outbox.batch-size:100}") int batchSize) {
        this.outbox = outbox;
        this.kafka = kafka;
        this.objectMapper = objectMapper;
        this.clock = clock;
        this.batchSize = batchSize;
    }

    @Scheduled(fixedDelayString = "${kestrel.outbox.poll-interval:PT1S}")
    @Transactional
    public void relay() {
        List<OutboxEvent> pending = outbox.lockUnsent(Limit.of(batchSize));
        for (OutboxEvent event : pending) {
            try {
                kafka.send(event.getTopic(), event.getAggregateId().toString(), envelope(event))
                        .get(SEND_TIMEOUT.toMillis(), TimeUnit.MILLISECONDS);
                event.markSent(clock.instant());
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                event.markFailed("interrupted");
                return;
            } catch (ExecutionException | TimeoutException | JsonProcessingException e) {
                log.warn("Outbox event {} ({}) not sent, will retry", event.getId(), event.getEventType(), e);
                event.markFailed(e.getMessage());
                return;
            }
        }
        if (!pending.isEmpty()) {
            log.debug("Relayed {} outbox events", pending.size());
        }
    }

    private String envelope(OutboxEvent event) throws JsonProcessingException {
        ObjectNode envelope = objectMapper.createObjectNode();
        envelope.put("type", event.getEventType());
        envelope.put("id", event.getId().toString());
        envelope.put("time", event.getCreatedAt().toString());
        envelope.set("data", objectMapper.readTree(event.getPayload()));
        return objectMapper.writeValueAsString(envelope);
    }
}
