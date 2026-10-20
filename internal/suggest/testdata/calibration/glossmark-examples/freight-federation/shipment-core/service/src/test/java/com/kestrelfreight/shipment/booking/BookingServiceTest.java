package com.kestrelfreight.shipment.booking;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.kestrelfreight.shipment.booking.BookShipmentRequest.AddressDto;
import com.kestrelfreight.shipment.booking.BookShipmentRequest.ParcelDto;
import com.kestrelfreight.shipment.carrier.CarrierSelector;
import com.kestrelfreight.shipment.events.OutboxEvent;
import com.kestrelfreight.shipment.events.OutboxRepository;
import com.kestrelfreight.shipment.model.Carrier;
import com.kestrelfreight.shipment.model.Shipment;
import com.kestrelfreight.shipment.model.Shipment.ServiceLevel;
import com.kestrelfreight.shipment.model.ShipmentRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.time.Clock;
import java.time.Instant;
import java.time.LocalDate;
import java.time.ZoneId;
import java.time.ZoneOffset;
import java.util.List;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyInt;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

// @gloss verifies .BookingHandler conf=0.9
@ExtendWith(MockitoExtension.class)
class BookingServiceTest {

    private static final Instant NOW = Instant.parse("2026-10-19T08:00:00Z");

    @Mock
    private ShipmentRepository shipments;
    @Mock
    private OutboxRepository outbox;
    @Mock
    private CarrierSelector carrierSelector;

    private BookingService service;

    private final Carrier northline = new Carrier("NLX", "Northline Express", List.of("M", "SK"), 30_000, true, 10);

    @BeforeEach
    void setUp() {
        service = new BookingService(shipments, outbox, carrierSelector,
                new DeliveryWindowPolicy(ZoneId.of("Europe/London")),
                new ObjectMapper().findAndRegisterModules(),
                Clock.fixed(NOW, ZoneOffset.UTC));
    }

    @Test
    void booksShipmentWithParcelsCarrierAndWindow() {
        when(carrierSelector.select("M1 1AE", 4_200, ServiceLevel.STANDARD)).thenReturn(Optional.of(northline));
        when(shipments.existsByTrackingNumber(anyString())).thenReturn(false);
        when(shipments.save(any(Shipment.class))).thenAnswer(invocation -> invocation.getArgument(0));

        ShipmentView view = service.book(request(ServiceLevel.STANDARD));

        assertThat(view.trackingNumber()).matches("KF\\d{11}");
        assertThat(view.status()).isEqualTo(Shipment.Status.BOOKED);
        assertThat(view.carrierCode()).isEqualTo("NLX");
        assertThat(view.parcelCount()).isEqualTo(2);
        assertThat(view.totalWeightGrams()).isEqualTo(5_400);
        assertThat(view.windowStart()).isEqualTo(Instant.parse("2026-10-22T09:00:00Z"));
        assertThat(view.windowEnd()).isEqualTo(Instant.parse("2026-10-22T11:00:00Z"));
        assertThat(view.bookedAt()).isEqualTo(NOW);
    }

    @Test
    void writesOutboxRowInTheSameUnitOfWork() {
        when(carrierSelector.select(anyString(), anyInt(), eq(ServiceLevel.STANDARD))).thenReturn(Optional.of(northline));
        when(shipments.save(any(Shipment.class))).thenAnswer(invocation -> invocation.getArgument(0));

        ShipmentView view = service.book(request(ServiceLevel.STANDARD));

        ArgumentCaptor<OutboxEvent> captor = ArgumentCaptor.forClass(OutboxEvent.class);
        verify(outbox).save(captor.capture());
        OutboxEvent row = captor.getValue();
        assertThat(row.getAggregateId()).isEqualTo(view.id());
        assertThat(row.getTopic()).isEqualTo("shipment.booked.v1");
        assertThat(row.getPayload()).contains(view.trackingNumber());
        assertThat(row.getSentAt()).isEmpty();
    }

    @Test
    void refusesBookingWhenNoCarrierCoversTheAddress() {
        when(carrierSelector.select(anyString(), anyInt(), any())).thenReturn(Optional.empty());

        assertThatThrownBy(() -> service.book(request(ServiceLevel.EXPRESS)))
                .isInstanceOf(BookingService.NoCarrierAvailableException.class)
                .hasMessageContaining("M1 1AE");
        verify(shipments, never()).save(any());
        verify(outbox, never()).save(any());
    }

    @Test
    void trackingNumberCheckDigitIsLuhn() {
        assertThat(BookingService.luhnCheckDigit("1234567890")).isEqualTo(3);
    }

    private static BookShipmentRequest request(ServiceLevel serviceLevel) {
        return new BookShipmentRequest(
                "shipper-4471",
                new AddressDto("Unit 4, Trafford Park Rd", "Manchester", "M17 1HH", "GB", 53.4655, -2.3204),
                new AddressDto("12 Piccadilly", "Manchester", "M1 1AE", "GB", 53.4810, -2.2374),
                List.of(new ParcelDto(4_200, 40, 30, 30), new ParcelDto(1_200, 25, 20, 10)),
                serviceLevel,
                LocalDate.of(2026, 10, 22),
                null);
    }
}
