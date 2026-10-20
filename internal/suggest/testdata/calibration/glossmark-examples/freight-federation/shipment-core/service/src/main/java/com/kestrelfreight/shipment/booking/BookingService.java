package com.kestrelfreight.shipment.booking;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.kestrelfreight.shipment.booking.DeliveryWindowPolicy.DeliveryWindow;
import com.kestrelfreight.shipment.carrier.CarrierSelector;
import com.kestrelfreight.shipment.events.OutboxEvent;
import com.kestrelfreight.shipment.events.OutboxRepository;
import com.kestrelfreight.shipment.events.ShipmentBookedEvent;
import com.kestrelfreight.shipment.model.Carrier;
import com.kestrelfreight.shipment.model.Parcel;
import com.kestrelfreight.shipment.model.Shipment;
import com.kestrelfreight.shipment.model.ShipmentRepository;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.security.SecureRandom;
import java.time.Clock;
import java.time.Instant;
import java.util.Optional;
import java.util.UUID;
import java.util.random.RandomGenerator;

// @gloss implements .BookingHandler conf=0.9
@Service
public class BookingService {

    public static class NoCarrierAvailableException extends RuntimeException {
        public NoCarrierAvailableException(String postcode, Shipment.ServiceLevel serviceLevel) {
            super("No active carrier covers " + postcode + " for " + serviceLevel);
        }
    }

    private static final int TRACKING_DIGITS = 10;

    private final ShipmentRepository shipments;
    private final OutboxRepository outbox;
    private final CarrierSelector carrierSelector;
    private final DeliveryWindowPolicy deliveryWindowPolicy;
    private final ObjectMapper objectMapper;
    private final Clock clock;
    private final RandomGenerator random = new SecureRandom();

    public BookingService(ShipmentRepository shipments, OutboxRepository outbox, CarrierSelector carrierSelector,
                          DeliveryWindowPolicy deliveryWindowPolicy, ObjectMapper objectMapper, Clock clock) {
        this.shipments = shipments;
        this.outbox = outbox;
        this.carrierSelector = carrierSelector;
        this.deliveryWindowPolicy = deliveryWindowPolicy;
        this.objectMapper = objectMapper;
        this.clock = clock;
    }

    @Transactional
    public ShipmentView book(BookShipmentRequest request) {
        Instant now = clock.instant();
        int heaviest = request.parcels().stream().mapToInt(BookShipmentRequest.ParcelDto::weightGrams).max().orElse(0);
        String deliveryPostcode = request.deliveryAddress().postcode();

        Carrier carrier = carrierSelector.select(deliveryPostcode, heaviest, request.serviceLevel())
                .orElseThrow(() -> new NoCarrierAvailableException(deliveryPostcode, request.serviceLevel()));

        DeliveryWindow window = deliveryWindowPolicy.promise(
                request.requestedDate(), request.serviceLevel(), request.preferredWindowStart(), now);

        Shipment shipment = new Shipment(
                nextTrackingNumber(),
                request.shipperId(),
                request.pickupAddress().toAddress(),
                request.deliveryAddress().toAddress(),
                request.serviceLevel(),
                carrier,
                window.start(),
                window.end(),
                now);
        request.parcels().forEach(p -> shipment.addParcel(
                new Parcel(p.weightGrams(), p.lengthCm(), p.widthCm(), p.heightCm())));

        Shipment saved = shipments.save(shipment);
        outbox.save(OutboxEvent.shipmentBooked(saved.getId(), toJson(ShipmentBookedEvent.from(saved)), now));
        return ShipmentView.from(saved);
    }

    @Transactional(readOnly = true)
    public Optional<ShipmentView> find(UUID shipmentId) {
        return shipments.findWithParcelsById(shipmentId).map(ShipmentView::from);
    }

    private String toJson(ShipmentBookedEvent event) {
        try {
            return objectMapper.writeValueAsString(event);
        } catch (JsonProcessingException e) {
            throw new IllegalStateException("Could not serialise " + ShipmentBookedEvent.TYPE, e);
        }
    }

    private String nextTrackingNumber() {
        String candidate;
        do {
            StringBuilder digits = new StringBuilder(TRACKING_DIGITS);
            for (int i = 0; i < TRACKING_DIGITS; i++) {
                digits.append(random.nextInt(10));
            }
            candidate = "KF" + digits + luhnCheckDigit(digits);
        } while (shipments.existsByTrackingNumber(candidate));
        return candidate;
    }

    static int luhnCheckDigit(CharSequence digits) {
        int sum = 0;
        boolean doubleIt = true;
        for (int i = digits.length() - 1; i >= 0; i--) {
            int d = digits.charAt(i) - '0';
            if (doubleIt) {
                d *= 2;
                if (d > 9) {
                    d -= 9;
                }
            }
            sum += d;
            doubleIt = !doubleIt;
        }
        return (10 - sum % 10) % 10;
    }
}
