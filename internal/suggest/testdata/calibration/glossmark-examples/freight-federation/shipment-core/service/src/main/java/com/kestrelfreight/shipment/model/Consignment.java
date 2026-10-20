package com.kestrelfreight.shipment.model;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.FetchType;
import jakarta.persistence.Id;
import jakarta.persistence.JoinColumn;
import jakarta.persistence.ManyToOne;
import jakarta.persistence.OneToMany;
import jakarta.persistence.Table;

import java.time.Instant;
import java.time.LocalDate;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.UUID;

/**
 * A manifest of shipments handed over to a single carrier on a single day.
 */
// @gloss represents frt:Consignment conf=0.95
@Entity
@Table(name = "consignments")
public class Consignment {

    public enum Status { OPEN, CLOSED }

    @Id
    private UUID id;

    @ManyToOne(fetch = FetchType.LAZY, optional = false)
    @JoinColumn(name = "carrier_code", nullable = false)
    private Carrier carrier;

    @Column(name = "manifest_date", nullable = false)
    private LocalDate manifestDate;

    @Column(name = "manifest_reference", nullable = false, unique = true)
    private String manifestReference;

    @Enumerated(EnumType.STRING)
    @Column(name = "status", nullable = false, length = 16)
    private Status status;

    @Column(name = "closed_at")
    private Instant closedAt;

    @OneToMany(mappedBy = "consignment")
    private List<Shipment> shipments = new ArrayList<>();

    protected Consignment() {
    }

    public Consignment(Carrier carrier, LocalDate manifestDate) {
        this.id = UUID.randomUUID();
        this.carrier = carrier;
        this.manifestDate = manifestDate;
        this.manifestReference = "%s-%s".formatted(carrier.getCode(), manifestDate);
        this.status = Status.OPEN;
    }

    public void add(Shipment shipment) {
        if (status != Status.OPEN) {
            throw new IllegalStateException("Consignment " + manifestReference + " is closed");
        }
        if (!shipment.getCarrier().getCode().equals(carrier.getCode())) {
            throw new IllegalArgumentException("Shipment " + shipment.getId() + " is not assigned to " + carrier.getCode());
        }
        shipments.add(shipment);
        shipment.assignTo(this);
    }

    public void close(Instant at) {
        this.status = Status.CLOSED;
        this.closedAt = at;
    }

    public UUID getId() {
        return id;
    }

    public Carrier getCarrier() {
        return carrier;
    }

    public LocalDate getManifestDate() {
        return manifestDate;
    }

    public String getManifestReference() {
        return manifestReference;
    }

    public Status getStatus() {
        return status;
    }

    public Instant getClosedAt() {
        return closedAt;
    }

    public List<Shipment> getShipments() {
        return Collections.unmodifiableList(shipments);
    }
}
