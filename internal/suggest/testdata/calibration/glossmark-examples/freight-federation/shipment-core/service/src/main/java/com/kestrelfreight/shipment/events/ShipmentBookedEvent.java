package com.kestrelfreight.shipment.events;

import com.kestrelfreight.shipment.model.Parcel;
import com.kestrelfreight.shipment.model.Shipment;

import java.time.Instant;
import java.util.List;
import java.util.UUID;

// @gloss represents frt:ShipmentBooked conf=0.95
public record ShipmentBookedEvent(
        UUID shipmentId,
        String trackingNumber,
        String shipperId,
        Address pickupAddress,
        Address deliveryAddress,
        List<ParcelLine> parcels,
        Shipment.ServiceLevel serviceLevel,
        String carrierCode,
        DeliveryWindow deliveryWindow,
        Instant bookedAt) {

    public static final String TYPE = "ShipmentBooked";
    public static final String TOPIC = "shipment.booked.v1";

    public record Address(String line1, String city, String postcode, String country, double lat, double lng) {

        static Address of(Shipment.Address address) {
            return new Address(address.line1(), address.city(), address.postcode(), address.country(),
                    address.lat(), address.lng());
        }
    }

    public record ParcelLine(UUID parcelId, int weightGrams, int lengthCm, int widthCm, int heightCm) {

        static ParcelLine of(Parcel parcel) {
            return new ParcelLine(parcel.getId(), parcel.getWeightGrams(), parcel.getLengthCm(),
                    parcel.getWidthCm(), parcel.getHeightCm());
        }
    }

    public record DeliveryWindow(Instant start, Instant end) {
    }

    public static ShipmentBookedEvent from(Shipment shipment) {
        return new ShipmentBookedEvent(
                shipment.getId(),
                shipment.getTrackingNumber(),
                shipment.getShipperId(),
                Address.of(shipment.getPickupAddress()),
                Address.of(shipment.getDeliveryAddress()),
                shipment.getParcels().stream().map(ParcelLine::of).toList(),
                shipment.getServiceLevel(),
                shipment.getCarrier().getCode(),
                new DeliveryWindow(shipment.getWindowStart(), shipment.getWindowEnd()),
                shipment.getBookedAt());
    }
}
