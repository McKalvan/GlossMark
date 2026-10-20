package com.kestrelfreight.shipment.metrics;

import io.micrometer.core.instrument.Gauge;
import io.micrometer.core.instrument.MeterRegistry;
import org.springframework.jdbc.core.simple.JdbcClient;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

import java.sql.Timestamp;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.util.concurrent.atomic.AtomicReference;

/**
 * On-time delivery rate: the share of deliveries in the last seven days that arrived inside
 * the window promised at booking.
 */
// @gloss implements .OnTimeRateCalculator conf=0.9
@Component
public class OnTimeRateCalculator {

    public static final double TARGET = 0.95;
    public static final Duration ROLLING_PERIOD = Duration.ofDays(7);

    public record OnTimeRate(Instant from, Instant to, long delivered, long onTime, double rate, double target) {

        public boolean meetsTarget() {
            return delivered > 0 && rate >= target;
        }
    }

    private final JdbcClient jdbc;
    private final Clock clock;
    private final AtomicReference<OnTimeRate> latest = new AtomicReference<>();

    public OnTimeRateCalculator(JdbcClient jdbc, Clock clock, MeterRegistry registry) {
        this.jdbc = jdbc;
        this.clock = clock;
        Gauge.builder("shipment.delivery.on_time_rate", latest, ref -> ref.get() == null ? Double.NaN : ref.get().rate())
                .description("Share of deliveries inside the promised window, 7-day rolling")
                .baseUnit("ratio")
                .register(registry);
        Gauge.builder("shipment.delivery.on_time_rate.target", () -> TARGET)
                .baseUnit("ratio")
                .register(registry);
    }

    public OnTimeRate current() {
        OnTimeRate cached = latest.get();
        return cached != null ? cached : refresh();
    }

    @Scheduled(fixedDelayString = "${kestrel.metrics.on-time-refresh:PT5M}")
    public OnTimeRate refresh() {
        Instant to = clock.instant();
        Instant from = to.minus(ROLLING_PERIOD);
        Counts counts = jdbc.sql("""
                        SELECT count(*)                                AS delivered,
                               count(*) FILTER (WHERE within_window)  AS on_time
                          FROM delivery_outcomes
                         WHERE delivered_at >= :from AND delivered_at < :to
                        """)
                .param("from", Timestamp.from(from))
                .param("to", Timestamp.from(to))
                .query(Counts.class)
                .single();
        OnTimeRate rate = rateOf(from, to, counts.delivered(), counts.onTime());
        latest.set(rate);
        return rate;
    }

    static OnTimeRate rateOf(Instant from, Instant to, long delivered, long onTime) {
        if (onTime > delivered) {
            throw new IllegalArgumentException("onTime " + onTime + " exceeds delivered " + delivered);
        }
        double rate = delivered == 0 ? 0.0 : (double) onTime / delivered;
        return new OnTimeRate(from, to, delivered, onTime, rate, TARGET);
    }

    record Counts(long delivered, long onTime) {
    }
}
