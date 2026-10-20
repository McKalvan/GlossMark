package com.kestrelfreight.shipment.model;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.FetchType;
import jakarta.persistence.Id;
import jakarta.persistence.JoinColumn;
import jakarta.persistence.OneToOne;
import jakarta.persistence.Table;

import java.time.Instant;
import java.util.UUID;

// @gloss represents frt:ShippingLabel conf=0.95
@Entity
@Table(name = "shipping_labels")
public class ShippingLabel {

    @Id
    private UUID id;

    @OneToOne(fetch = FetchType.LAZY, optional = false)
    @JoinColumn(name = "shipment_id", nullable = false, unique = true)
    private Shipment shipment;

    @Column(name = "tracking_number", nullable = false, length = 13)
    private String trackingNumber;

    @Column(name = "format", nullable = false)
    private String format;

    @Column(name = "zpl", nullable = false, columnDefinition = "text")
    private String zpl;

    @Column(name = "rendered_at", nullable = false)
    private Instant renderedAt;

    protected ShippingLabel() {
    }

    public ShippingLabel(Shipment shipment, String format, String zpl, Instant renderedAt) {
        this.id = UUID.randomUUID();
        this.shipment = shipment;
        this.trackingNumber = shipment.getTrackingNumber();
        this.format = format;
        this.zpl = zpl;
        this.renderedAt = renderedAt;
    }

    public UUID getId() {
        return id;
    }

    public Shipment getShipment() {
        return shipment;
    }

    public String getTrackingNumber() {
        return trackingNumber;
    }

    public String getFormat() {
        return format;
    }

    public String getZpl() {
        return zpl;
    }

    public Instant getRenderedAt() {
        return renderedAt;
    }
}
