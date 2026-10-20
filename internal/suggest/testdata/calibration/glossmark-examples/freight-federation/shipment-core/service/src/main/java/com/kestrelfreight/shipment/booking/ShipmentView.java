package com.kestrelfreight.shipment.booking;

import com.kestrelfreight.shipment.model.Shipment;

import java.time.Instant;
import java.util.UUID;

public record ShipmentView(
        UUID id,
        String trackingNumber,
        Shipment.Status status,
        Shipment.ServiceLevel serviceLevel,
        String carrierCode,
        Instant windowStart,
        Instant windowEnd,
        int parcelCount,
        int totalWeightGrams,
        Instant bookedAt,
        Instant deliveredAt) {

    public static ShipmentView from(Shipment shipment) {
        return new ShipmentView(
                shipment.getId(),
                shipment.getTrackingNumber(),
                shipment.getStatus(),
                shipment.getServiceLevel(),
                shipment.getCarrier().getCode(),
                shipment.getWindowStart(),
                shipment.getWindowEnd(),
                shipment.getParcels().size(),
                shipment.totalWeightGrams(),
                shipment.getBookedAt(),
                shipment.getDeliveredAt().orElse(null));
    }
}
