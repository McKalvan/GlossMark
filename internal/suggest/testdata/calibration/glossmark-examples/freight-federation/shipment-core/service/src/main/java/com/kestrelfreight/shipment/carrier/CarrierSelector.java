package com.kestrelfreight.shipment.carrier;

import com.kestrelfreight.shipment.model.Carrier;
import com.kestrelfreight.shipment.model.Shipment.ServiceLevel;
import org.springframework.stereotype.Component;

import java.util.Comparator;
import java.util.Optional;

/**
 * Chooses the carrier for a shipment: it must cover the delivery postcode, accept the heaviest parcel,
 * and run express services when the shipment is EXPRESS. Ties go to the lowest priority number.
 */
// @gloss implements .CarrierSelector conf=0.95
@Component
public class CarrierSelector {

    private final CarrierRepository carriers;

    public CarrierSelector(CarrierRepository carriers) {
        this.carriers = carriers;
    }

    public Optional<Carrier> select(String deliveryPostcode, int heaviestParcelGrams, ServiceLevel serviceLevel) {
        return carriers.findByActiveTrueOrderByPriorityAscCodeAsc().stream()
                .filter(carrier -> carrier.covers(deliveryPostcode))
                .filter(carrier -> carrier.accepts(heaviestParcelGrams))
                .filter(carrier -> serviceLevel != ServiceLevel.EXPRESS || carrier.isSupportsExpress())
                .min(Comparator.comparingInt(Carrier::getPriority).thenComparing(Carrier::getCode));
    }
}
