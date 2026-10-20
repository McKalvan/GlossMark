package com.kestrelfreight.shipment.model;

import jakarta.persistence.AttributeOverride;
import jakarta.persistence.AttributeOverrides;
import jakarta.persistence.CascadeType;
import jakarta.persistence.Column;
import jakarta.persistence.Embeddable;
import jakarta.persistence.Embedded;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.FetchType;
import jakarta.persistence.Id;
import jakarta.persistence.JoinColumn;
import jakarta.persistence.ManyToOne;
import jakarta.persistence.OneToMany;
import jakarta.persistence.OneToOne;
import jakarta.persistence.OrderBy;
import jakarta.persistence.Table;
import jakarta.persistence.Version;

import java.time.Instant;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Optional;
import java.util.UUID;

// @gloss represents frt:Shipment conf=0.95
@Entity
@Table(name = "shipments")
public class Shipment {

    public enum Status { BOOKED, PICKED_UP, DELIVERED, CANCELLED }

    public enum ServiceLevel { STANDARD, EXPRESS }

    @Embeddable
    public record Address(String line1, String city, String postcode, String country, double lat, double lng) {
    }

    @Id
    private UUID id;

    @Column(name = "tracking_number", nullable = false, unique = true, length = 13)
    private String trackingNumber;

    @Column(name = "shipper_id", nullable = false)
    private String shipperId;

    @Embedded
    @AttributeOverrides({
            @AttributeOverride(name = "line1", column = @Column(name = "pickup_line1")),
            @AttributeOverride(name = "city", column = @Column(name = "pickup_city")),
            @AttributeOverride(name = "postcode", column = @Column(name = "pickup_postcode")),
            @AttributeOverride(name = "country", column = @Column(name = "pickup_country")),
            @AttributeOverride(name = "lat", column = @Column(name = "pickup_lat")),
            @AttributeOverride(name = "lng", column = @Column(name = "pickup_lng"))
    })
    private Address pickupAddress;

    @Embedded
    @AttributeOverrides({
            @AttributeOverride(name = "line1", column = @Column(name = "delivery_line1")),
            @AttributeOverride(name = "city", column = @Column(name = "delivery_city")),
            @AttributeOverride(name = "postcode", column = @Column(name = "delivery_postcode")),
            @AttributeOverride(name = "country", column = @Column(name = "delivery_country")),
            @AttributeOverride(name = "lat", column = @Column(name = "delivery_lat")),
            @AttributeOverride(name = "lng", column = @Column(name = "delivery_lng"))
    })
    private Address deliveryAddress;

    @Enumerated(EnumType.STRING)
    @Column(name = "service_level", nullable = false, length = 16)
    private ServiceLevel serviceLevel;

    @Enumerated(EnumType.STRING)
    @Column(name = "status", nullable = false, length = 16)
    private Status status;

    @ManyToOne(fetch = FetchType.LAZY, optional = false)
    @JoinColumn(name = "carrier_code", nullable = false)
    private Carrier carrier;

    @ManyToOne(fetch = FetchType.LAZY)
    @JoinColumn(name = "consignment_id")
    private Consignment consignment;

    @OneToMany(mappedBy = "shipment", cascade = CascadeType.ALL, orphanRemoval = true)
    @OrderBy("sequence ASC")
    private List<Parcel> parcels = new ArrayList<>();

    @OneToOne(mappedBy = "shipment", cascade = CascadeType.ALL, fetch = FetchType.LAZY)
    private ShippingLabel label;

    @Column(name = "window_start", nullable = false)
    private Instant windowStart;

    @Column(name = "window_end", nullable = false)
    private Instant windowEnd;

    @Column(name = "booked_at", nullable = false)
    private Instant bookedAt;

    @Column(name = "picked_up_at")
    private Instant pickedUpAt;

    @Column(name = "delivered_at")
    private Instant deliveredAt;

    @Version
    private long version;

    protected Shipment() {
    }

    public Shipment(String trackingNumber, String shipperId, Address pickupAddress, Address deliveryAddress,
                    ServiceLevel serviceLevel, Carrier carrier, Instant windowStart, Instant windowEnd,
                    Instant bookedAt) {
        this.id = UUID.randomUUID();
        this.trackingNumber = trackingNumber;
        this.shipperId = shipperId;
        this.pickupAddress = pickupAddress;
        this.deliveryAddress = deliveryAddress;
        this.serviceLevel = serviceLevel;
        this.carrier = carrier;
        this.windowStart = windowStart;
        this.windowEnd = windowEnd;
        this.bookedAt = bookedAt;
        this.status = Status.BOOKED;
    }

    public void addParcel(Parcel parcel) {
        parcel.attachTo(this, parcels.size() + 1);
        parcels.add(parcel);
    }

    public void attachLabel(ShippingLabel label) {
        this.label = label;
    }

    void assignTo(Consignment consignment) {
        this.consignment = consignment;
    }

    public boolean markPickedUp(Instant at) {
        if (status != Status.BOOKED) {
            return false;
        }
        this.status = Status.PICKED_UP;
        this.pickedUpAt = at;
        return true;
    }

    public boolean markDelivered(Instant at) {
        if (status == Status.DELIVERED || status == Status.CANCELLED) {
            return false;
        }
        this.status = Status.DELIVERED;
        this.deliveredAt = at;
        return true;
    }

    public void cancel() {
        if (status != Status.BOOKED) {
            throw new IllegalStateException("Only booked shipments can be cancelled; " + id + " is " + status);
        }
        this.status = Status.CANCELLED;
    }

    public boolean isWithinWindow(Instant deliveredAt) {
        return !deliveredAt.isBefore(windowStart) && !deliveredAt.isAfter(windowEnd);
    }

    public int totalWeightGrams() {
        return parcels.stream().mapToInt(Parcel::getWeightGrams).sum();
    }

    public int heaviestParcelGrams() {
        return parcels.stream().mapToInt(Parcel::getWeightGrams).max().orElse(0);
    }

    public UUID getId() {
        return id;
    }

    public String getTrackingNumber() {
        return trackingNumber;
    }

    public String getShipperId() {
        return shipperId;
    }

    public Address getPickupAddress() {
        return pickupAddress;
    }

    public Address getDeliveryAddress() {
        return deliveryAddress;
    }

    public ServiceLevel getServiceLevel() {
        return serviceLevel;
    }

    public Status getStatus() {
        return status;
    }

    public Carrier getCarrier() {
        return carrier;
    }

    public Optional<Consignment> getConsignment() {
        return Optional.ofNullable(consignment);
    }

    public List<Parcel> getParcels() {
        return Collections.unmodifiableList(parcels);
    }

    public Optional<ShippingLabel> getLabel() {
        return Optional.ofNullable(label);
    }

    public Instant getWindowStart() {
        return windowStart;
    }

    public Instant getWindowEnd() {
        return windowEnd;
    }

    public Instant getBookedAt() {
        return bookedAt;
    }

    public Optional<Instant> getPickedUpAt() {
        return Optional.ofNullable(pickedUpAt);
    }

    public Optional<Instant> getDeliveredAt() {
        return Optional.ofNullable(deliveredAt);
    }
}
