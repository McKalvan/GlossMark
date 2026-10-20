package com.kestrelfreight.shipment.booking;

import com.kestrelfreight.shipment.booking.DeliveryWindowPolicy.DeliveryWindow;
import com.kestrelfreight.shipment.model.Shipment.ServiceLevel;
import org.junit.jupiter.api.Test;

import java.time.Duration;
import java.time.Instant;
import java.time.LocalDate;
import java.time.LocalTime;
import java.time.ZoneId;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

// @gloss verifies .DeliveryWindowPolicy conf=0.64
class DeliveryWindowPolicyTest {

    private static final ZoneId LONDON = ZoneId.of("Europe/London");

    private final DeliveryWindowPolicy policy = new DeliveryWindowPolicy(LONDON);

    @Test
    void windowIsAlwaysTwoHours() {
        assertThat(DeliveryWindowPolicy.WINDOW_LENGTH).isEqualTo(Duration.ofHours(2));

        DeliveryWindow window = policy.promise(LocalDate.of(2026, 10, 21), ServiceLevel.STANDARD, null,
                Instant.parse("2026-10-19T08:00:00Z"));

        assertThat(Duration.between(window.start(), window.end())).isEqualTo(DeliveryWindowPolicy.WINDOW_LENGTH);
    }

    @Test
    void expressBookedBeforeCutoffIsDeliveredSameAfternoon() {
        // 10:30 BST on Monday 19 October
        DeliveryWindow window = policy.promise(LocalDate.of(2026, 10, 19), ServiceLevel.EXPRESS, null,
                Instant.parse("2026-10-19T09:30:00Z"));

        assertThat(window.start()).isEqualTo(Instant.parse("2026-10-19T14:00:00Z"));
        assertThat(window.end()).isEqualTo(Instant.parse("2026-10-19T16:00:00Z"));
    }

    @Test
    void expressBookedAfterCutoffRollsToNextWorkingDay() {
        // 11:30 BST, past the 11:00 express cutoff
        DeliveryWindow window = policy.promise(LocalDate.of(2026, 10, 19), ServiceLevel.EXPRESS, null,
                Instant.parse("2026-10-19T10:30:00Z"));

        assertThat(window.start()).isEqualTo(Instant.parse("2026-10-20T09:00:00Z"));
    }

    @Test
    void standardNeedsTwoWorkingDaysAndCrossesTheClockChange() {
        // Booked Friday 23 October for the same day; earliest is Tuesday 27 October, after BST ends
        DeliveryWindow window = policy.promise(LocalDate.of(2026, 10, 23), ServiceLevel.STANDARD, null,
                Instant.parse("2026-10-23T08:00:00Z"));

        assertThat(window.start()).isEqualTo(Instant.parse("2026-10-27T10:00:00Z"));
        assertThat(window.end()).isEqualTo(Instant.parse("2026-10-27T12:00:00Z"));
    }

    @Test
    void weekendRequestRollsToMonday() {
        DeliveryWindow window = policy.promise(LocalDate.of(2026, 10, 31), ServiceLevel.STANDARD,
                LocalTime.of(13, 0), Instant.parse("2026-10-19T08:00:00Z"));

        assertThat(window.start()).isEqualTo(Instant.parse("2026-11-02T13:00:00Z"));
    }

    @Test
    void preferredStartIsClampedToOperatingHours() {
        Instant bookedAt = Instant.parse("2026-10-19T08:00:00Z");
        LocalDate thursday = LocalDate.of(2026, 10, 22);

        assertThat(policy.promise(thursday, ServiceLevel.STANDARD, LocalTime.of(7, 15), bookedAt).start())
                .isEqualTo(Instant.parse("2026-10-22T07:00:00Z"));
        assertThat(policy.promise(thursday, ServiceLevel.STANDARD, LocalTime.of(19, 40), bookedAt).start())
                .isEqualTo(Instant.parse("2026-10-22T17:00:00Z"));
    }

    @Test
    void rejectsRequestedDateInThePast() {
        assertThatThrownBy(() -> policy.promise(LocalDate.of(2026, 10, 18), ServiceLevel.STANDARD, null,
                Instant.parse("2026-10-19T08:00:00Z")))
                .isInstanceOf(IllegalArgumentException.class);
    }
}
