package com.kestrelfreight.shipment.model;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.FetchType;
import jakarta.persistence.Id;
import jakarta.persistence.JoinColumn;
import jakarta.persistence.ManyToOne;
import jakarta.persistence.Table;

import java.util.UUID;

// @gloss represents frt:Parcel conf=0.95
@Entity
@Table(name = "parcels")
public class Parcel {

    @Id
    private UUID id;

    @ManyToOne(fetch = FetchType.LAZY, optional = false)
    @JoinColumn(name = "shipment_id", nullable = false)
    private Shipment shipment;

    @Column(name = "seq", nullable = false)
    private int sequence;

    @Column(name = "weight_grams", nullable = false)
    private int weightGrams;

    @Column(name = "length_cm", nullable = false)
    private int lengthCm;

    @Column(name = "width_cm", nullable = false)
    private int widthCm;

    @Column(name = "height_cm", nullable = false)
    private int heightCm;

    protected Parcel() {
    }

    public Parcel(int weightGrams, int lengthCm, int widthCm, int heightCm) {
        this.id = UUID.randomUUID();
        this.weightGrams = weightGrams;
        this.lengthCm = lengthCm;
        this.widthCm = widthCm;
        this.heightCm = heightCm;
    }

    void attachTo(Shipment shipment, int sequence) {
        this.shipment = shipment;
        this.sequence = sequence;
    }

    public UUID getId() {
        return id;
    }

    public Shipment getShipment() {
        return shipment;
    }

    public int getSequence() {
        return sequence;
    }

    public int getWeightGrams() {
        return weightGrams;
    }

    public int getLengthCm() {
        return lengthCm;
    }

    public int getWidthCm() {
        return widthCm;
    }

    public int getHeightCm() {
        return heightCm;
    }
}
