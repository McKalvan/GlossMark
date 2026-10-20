/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2014 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.service;

import java.io.FileInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.math.BigDecimal;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;

import org.yaml.snakeyaml.Yaml;

/** Reads config/circulation.yaml once at startup (CIRC_CONFIG points at it). */
public class CircConfig {

    private final Map<String, Object> root;

    @SuppressWarnings("unchecked")
    public CircConfig(String path) throws IOException {
        InputStream in = new FileInputStream(path);
        try {
            root = (Map<String, Object>) new Yaml().load(in);
        } finally {
            in.close();
        }
    }

    @SuppressWarnings("unchecked")
    private Map<String, Object> section(String name) {
        return (Map<String, Object>) root.get(name);
    }

    public int graceDays() {
        return ((Number) section("fines").get("grace_days")).intValue();
    }

    public BigDecimal dailyRate(String format) {
        @SuppressWarnings("unchecked")
        Map<String, Object> rates = (Map<String, Object>) section("fines").get("daily_rate");
        Object r = rates.containsKey(format) ? rates.get(format) : rates.get("DEFAULT");
        return new BigDecimal(String.valueOf(r));
    }

    public BigDecimal fineCapFlat() {
        return new BigDecimal(String.valueOf(section("fines").get("cap_flat")));
    }

    public BigDecimal blockThreshold() {
        return new BigDecimal(String.valueOf(section("patrons").get("block_threshold")));
    }

    public int maxRenewals() {
        return ((Number) section("loans").get("max_renewals")).intValue();
    }

    public int holdPickupDays() {
        return ((Number) section("holds").get("pickup_days")).intValue();
    }

    @SuppressWarnings("unchecked")
    public Set<String> closedDays() {
        return new HashSet<String>((List<String>) section("calendar").get("closed"));
    }
}
