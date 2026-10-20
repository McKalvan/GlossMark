package com.kestrelfreight.shipment.carrier;

import com.kestrelfreight.shipment.model.Carrier;
import com.kestrelfreight.shipment.model.Shipment.ServiceLevel;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

// @gloss verifies .CarrierSelector conf=0.55
class CarrierSelectorTest {

    private final CarrierRepository repository = mock(CarrierRepository.class);
    private final CarrierSelector selector = new CarrierSelector(repository);

    private final Carrier northline = new Carrier("NLX", "Northline Express", List.of("M", "SK"), 20_000, true, 10);
    private final Carrier pennine = new Carrier("PEN", "Pennine Parcels", List.of("M", "OL", "BD"), 31_500, false, 20);
    private final Carrier capital = new Carrier("CAP", "Capital Couriers", List.of("EC", "WC", "N"), 25_000, true, 5);

    @BeforeEach
    void setUp() {
        when(repository.findByActiveTrueOrderByPriorityAscCodeAsc()).thenReturn(List.of(capital, northline, pennine));
    }

    @Test
    void picksHighestPriorityCarrierCoveringThePostcode() {
        assertThat(selector.select("M1 1AE", 2_000, ServiceLevel.STANDARD))
                .map(Carrier::getCode).contains("NLX");
    }

    @Test
    void matchesPostcodesIgnoringSpacesAndCase() {
        assertThat(selector.select("ec1a 1bb", 2_000, ServiceLevel.STANDARD))
                .map(Carrier::getCode).contains("CAP");
    }

    @Test
    void skipsCarriersThatCannotTakeTheHeaviestParcel() {
        assertThat(selector.select("M1 1AE", 25_000, ServiceLevel.STANDARD))
                .map(Carrier::getCode).contains("PEN");
    }

    @Test
    void expressRequiresAnExpressCarrier() {
        assertThat(selector.select("OL1 1AA", 2_000, ServiceLevel.EXPRESS)).isEmpty();
        assertThat(selector.select("OL1 1AA", 2_000, ServiceLevel.STANDARD))
                .map(Carrier::getCode).contains("PEN");
    }

    @Test
    void returnsEmptyWhenNoCarrierCoversThePostcode() {
        assertThat(selector.select("IV1 1AA", 1_000, ServiceLevel.STANDARD)).isEmpty();
    }
}
