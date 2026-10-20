package com.kestrelfreight.shipment.booking;

import com.kestrelfreight.shipment.model.Shipment;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.boot.test.mock.mockito.MockBean;
import org.springframework.http.MediaType;
import org.springframework.test.web.servlet.MockMvc;

import java.time.Instant;
import java.util.Optional;
import java.util.UUID;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.header;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

// @gloss verifies .BookingHandler conf=0.85
@WebMvcTest(ShipmentController.class)
class ShipmentControllerIT {

    private static final UUID SHIPMENT_ID = UUID.fromString("0f8e2c3a-5b1d-4c7e-9a64-2d1f0b7e8c11");

    private static final String VALID_BOOKING = """
            {
              "shipperId": "shipper-4471",
              "pickupAddress": {"line1": "Unit 4, Trafford Park Rd", "city": "Manchester", "postcode": "M17 1HH",
                                "country": "GB", "lat": 53.4655, "lng": -2.3204},
              "deliveryAddress": {"line1": "12 Piccadilly", "city": "Manchester", "postcode": "M1 1AE",
                                  "country": "GB", "lat": 53.4810, "lng": -2.2374},
              "parcels": [{"weightGrams": 4200, "lengthCm": 40, "widthCm": 30, "heightCm": 30}],
              "serviceLevel": "STANDARD",
              "requestedDate": "2099-01-06"
            }
            """;

    @Autowired
    private MockMvc mvc;

    @MockBean
    private BookingService bookingService;

    @Test
    void bookingReturnsCreatedWithLocation() throws Exception {
        when(bookingService.book(any())).thenReturn(view());

        mvc.perform(post("/shipments").contentType(MediaType.APPLICATION_JSON).content(VALID_BOOKING))
                .andExpect(status().isCreated())
                .andExpect(header().string("Location", "http://localhost/shipments/" + SHIPMENT_ID))
                .andExpect(jsonPath("$.trackingNumber").value("KF12345678903"))
                .andExpect(jsonPath("$.status").value("BOOKED"))
                .andExpect(jsonPath("$.carrierCode").value("NLX"));
    }

    @Test
    void bookingWithoutParcelsIsRejected() throws Exception {
        String noParcels = VALID_BOOKING.replaceFirst("\"parcels\": \\[.*],", "\"parcels\": [],");

        mvc.perform(post("/shipments").contentType(MediaType.APPLICATION_JSON).content(noParcels))
                .andExpect(status().isBadRequest());
    }

    @Test
    void bookingWithNoCarrierIsUnprocessable() throws Exception {
        when(bookingService.book(any()))
                .thenThrow(new BookingService.NoCarrierAvailableException("M1 1AE", Shipment.ServiceLevel.STANDARD));

        mvc.perform(post("/shipments").contentType(MediaType.APPLICATION_JSON).content(VALID_BOOKING))
                .andExpect(status().isUnprocessableEntity())
                .andExpect(jsonPath("$.title").value("No carrier available"));
    }

    @Test
    void getReturnsShipment() throws Exception {
        when(bookingService.find(SHIPMENT_ID)).thenReturn(Optional.of(view()));

        mvc.perform(get("/shipments/{id}", SHIPMENT_ID))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.id").value(SHIPMENT_ID.toString()))
                .andExpect(jsonPath("$.windowStart").value("2099-01-06T09:00:00Z"));
    }

    @Test
    void getUnknownShipmentIsNotFound() throws Exception {
        when(bookingService.find(any())).thenReturn(Optional.empty());

        mvc.perform(get("/shipments/{id}", UUID.randomUUID()))
                .andExpect(status().isNotFound());
    }

    private static ShipmentView view() {
        return new ShipmentView(SHIPMENT_ID, "KF12345678903", Shipment.Status.BOOKED, Shipment.ServiceLevel.STANDARD,
                "NLX", Instant.parse("2099-01-06T09:00:00Z"), Instant.parse("2099-01-06T11:00:00Z"),
                1, 4_200, Instant.parse("2099-01-02T08:00:00Z"), null);
    }
}
