package com.kestrelfreight.shipment.model;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;

import java.util.Arrays;
import java.util.List;
import java.util.Locale;

// @gloss represents frt:Carrier conf=0.95
@Entity
@Table(name = "carriers")
public class Carrier {

    @Id
    @Column(name = "code", length = 16)
    private String code;

    @Column(name = "name", nullable = false)
    private String name;

    @Column(name = "postcode_prefixes", nullable = false, columnDefinition = "text[]")
    private String[] postcodePrefixes;

    @Column(name = "max_parcel_weight_grams", nullable = false)
    private int maxParcelWeightGrams;

    @Column(name = "supports_express", nullable = false)
    private boolean supportsExpress;

    @Column(name = "priority", nullable = false)
    private int priority;

    @Column(name = "active", nullable = false)
    private boolean active;

    protected Carrier() {
    }

    public Carrier(String code, String name, List<String> postcodePrefixes, int maxParcelWeightGrams,
                   boolean supportsExpress, int priority) {
        this.code = code;
        this.name = name;
        this.postcodePrefixes = postcodePrefixes.toArray(String[]::new);
        this.maxParcelWeightGrams = maxParcelWeightGrams;
        this.supportsExpress = supportsExpress;
        this.priority = priority;
        this.active = true;
    }

    public boolean covers(String postcode) {
        String normalised = postcode.replace(" ", "").toUpperCase(Locale.ROOT);
        return Arrays.stream(postcodePrefixes).anyMatch(normalised::startsWith);
    }

    public boolean accepts(int parcelWeightGrams) {
        return parcelWeightGrams <= maxParcelWeightGrams;
    }

    public String getCode() {
        return code;
    }

    public String getName() {
        return name;
    }

    public List<String> getPostcodePrefixes() {
        return List.of(postcodePrefixes);
    }

    public int getMaxParcelWeightGrams() {
        return maxParcelWeightGrams;
    }

    public boolean isSupportsExpress() {
        return supportsExpress;
    }

    public int getPriority() {
        return priority;
    }

    public boolean isActive() {
        return active;
    }

    public void deactivate() {
        this.active = false;
    }
}
