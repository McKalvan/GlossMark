package com.kestrelfreight.shipment.metrics;

import com.kestrelfreight.shipment.metrics.OnTimeRateCalculator.OnTimeRate;
import org.junit.jupiter.api.Test;

import java.time.Duration;
import java.time.Instant;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.assertj.core.api.Assertions.within;

// @gloss verifies .OnTimeRateCalculator conf=0.64
class OnTimeRateCalculatorTest {

    private static final Instant TO = Instant.parse("2026-10-19T00:00:00Z");
    private static final Instant FROM = TO.minus(OnTimeRateCalculator.ROLLING_PERIOD);

    @Test
    void rollingPeriodIsSevenDaysAndTargetIsNinetyFivePercent() {
        assertThat(OnTimeRateCalculator.ROLLING_PERIOD).isEqualTo(Duration.ofDays(7));
        assertThat(OnTimeRateCalculator.TARGET).isEqualTo(0.95);
    }

    @Test
    void rateIsShareOfDeliveriesInsideTheirWindow() {
        OnTimeRate rate = OnTimeRateCalculator.rateOf(FROM, TO, 400, 386);

        assertThat(rate.rate()).isCloseTo(0.965, within(1e-9));
        assertThat(rate.meetsTarget()).isTrue();
    }

    @Test
    void belowTargetIsReported() {
        OnTimeRate rate = OnTimeRateCalculator.rateOf(FROM, TO, 200, 188);

        assertThat(rate.rate()).isCloseTo(0.94, within(1e-9));
        assertThat(rate.meetsTarget()).isFalse();
    }

    @Test
    void exactlyOnTargetMeetsIt() {
        assertThat(OnTimeRateCalculator.rateOf(FROM, TO, 20, 19).meetsTarget()).isTrue();
    }

    @Test
    void noDeliveriesNeverMeetsTarget() {
        OnTimeRate rate = OnTimeRateCalculator.rateOf(FROM, TO, 0, 0);

        assertThat(rate.rate()).isZero();
        assertThat(rate.meetsTarget()).isFalse();
    }

    @Test
    void rejectsMoreOnTimeThanDelivered() {
        assertThatThrownBy(() -> OnTimeRateCalculator.rateOf(FROM, TO, 3, 4))
                .isInstanceOf(IllegalArgumentException.class);
    }
}
