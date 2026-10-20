package com.kestrelfreight.shipment.events;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.kestrelfreight.shipment.model.Shipment;
import com.kestrelfreight.shipment.model.ShipmentRepository;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.jdbc.core.simple.JdbcClient;
import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.kafka.support.KafkaHeaders;
import org.springframework.messaging.handler.annotation.Header;
import org.springframework.messaging.handler.annotation.Payload;
import org.springframework.stereotype.Component;
import org.springframework.transaction.annotation.Transactional;

import java.io.IOException;
import java.sql.Timestamp;
import java.time.Instant;
import java.util.UUID;

/**
 * Keeps the shipment status in step with what dispatch and tracking observe on the road,
 * and records each delivery outcome against the promised window.
 */
// @gloss implements .StatusProjector conf=0.9
@Component
public class ShipmentStatusListener {

    public static final String PICKED_UP_TOPIC = "shipment.picked-up.v1";
    public static final String DELIVERED_TOPIC = "shipment.delivered.v1";

    private static final Logger log = LoggerFactory.getLogger(ShipmentStatusListener.class);

    private final ShipmentRepository shipments;
    private final JdbcClient jdbc;
    private final ObjectMapper objectMapper;

    public ShipmentStatusListener(ShipmentRepository shipments, JdbcClient jdbc, ObjectMapper objectMapper) {
        this.shipments = shipments;
        this.jdbc = jdbc;
        this.objectMapper = objectMapper;
    }

    @KafkaListener(topics = {PICKED_UP_TOPIC, DELIVERED_TOPIC}, groupId = "shipment-core")
    @Transactional
    public void onStatusEvent(@Payload String message, @Header(KafkaHeaders.RECEIVED_TOPIC) String topic)
            throws IOException {
        JsonNode data = objectMapper.readTree(message).path("data");
        UUID shipmentId = UUID.fromString(data.path("shipmentId").asText());

        shipments.findById(shipmentId).ifPresentOrElse(
                shipment -> {
                    switch (topic) {
                        case PICKED_UP_TOPIC -> pickedUp(shipment, Instant.parse(data.path("pickedUpAt").asText()));
                        case DELIVERED_TOPIC -> delivered(shipment, data);
                        default -> log.warn("Ignoring message from unexpected topic {}", topic);
                    }
                },
                () -> log.warn("Status event on {} for unknown shipment {}", topic, shipmentId));
    }

    private void pickedUp(Shipment shipment, Instant pickedUpAt) {
        if (!shipment.markPickedUp(pickedUpAt)) {
            log.debug("Shipment {} already {}, pickup ignored", shipment.getId(), shipment.getStatus());
        }
    }

    private void delivered(Shipment shipment, JsonNode data) {
        Instant deliveredAt = Instant.parse(data.path("deliveredAt").asText());
        if (!shipment.markDelivered(deliveredAt)) {
            log.debug("Shipment {} already {}, delivery ignored", shipment.getId(), shipment.getStatus());
            return;
        }
        boolean withinWindow = shipment.isWithinWindow(deliveredAt);
        if (data.has("withinWindow") && data.get("withinWindow").asBoolean() != withinWindow) {
            log.info("Tracking reported withinWindow={} for {} but promise says {}; using the promise",
                    data.get("withinWindow").asBoolean(), shipment.getTrackingNumber(), withinWindow);
        }
        jdbc.sql("""
                        INSERT INTO delivery_outcomes
                            (shipment_id, carrier_code, service_level, window_start, window_end,
                             delivered_at, within_window, proof_kind)
                        VALUES (:shipmentId, :carrierCode, :serviceLevel, :windowStart, :windowEnd,
                                :deliveredAt, :withinWindow, :proofKind)
                        ON CONFLICT (shipment_id) DO NOTHING
                        """)
                .param("shipmentId", shipment.getId())
                .param("carrierCode", shipment.getCarrier().getCode())
                .param("serviceLevel", shipment.getServiceLevel().name())
                .param("windowStart", Timestamp.from(shipment.getWindowStart()))
                .param("windowEnd", Timestamp.from(shipment.getWindowEnd()))
                .param("deliveredAt", Timestamp.from(deliveredAt))
                .param("withinWindow", withinWindow)
                .param("proofKind", data.path("proof").path("kind").asText(null))
                .update();
    }
}
