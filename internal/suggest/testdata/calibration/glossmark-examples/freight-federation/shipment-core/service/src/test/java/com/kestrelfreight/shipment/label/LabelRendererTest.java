package com.kestrelfreight.shipment.label;

import com.kestrelfreight.shipment.model.Carrier;
import com.kestrelfreight.shipment.model.Parcel;
import com.kestrelfreight.shipment.model.Shipment;
import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.time.ZoneId;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

// @gloss verifies .LabelRenderer, .LabelSpec conf=0.9
class LabelRendererTest {

    private final LabelRenderer renderer = new LabelRenderer(ZoneId.of("Europe/London"));

    @Test
    void rendersFourBySixLabelAt203Dpi() {
        String zpl = renderer.render(shipment());

        assertThat(zpl).startsWith("^XA").endsWith("^XZ\n");
        assertThat(zpl).contains("^PW812", "^LL1218");
        assertThat(LabelSpec.FORMAT).isEqualTo("ZPL II, 4x6 in, 203 dpi");
    }

    @Test
    void encodesTrackingNumberAsCode128() {
        String zpl = renderer.render(shipment());

        assertThat(zpl).contains("^BCN,160,Y,N,N,A^FDKF12345678903^FS");
    }

    @Test
    void printsDeliveryAddressWindowAndWeight() {
        String zpl = renderer.render(shipment());

        assertThat(zpl).contains("^FD12 Piccadilly^FS", "^FDM1 1AE  GB^FS");
        assertThat(zpl).contains("Deliver Thu 22 Oct 10:00-12:00");
        assertThat(zpl).contains("Parcels: 2   Weight: 5.4 kg");
    }

    @Test
    void refusesMalformedTrackingNumbers() {
        assertThatThrownBy(() -> LabelSpec.code128("KF123", 0, 0))
                .isInstanceOf(IllegalArgumentException.class);
    }

    private static Shipment shipment() {
        Carrier carrier = new Carrier("NLX", "Northline Express", List.of("M"), 30_000, true, 10);
        Shipment shipment = new Shipment(
                "KF12345678903",
                "shipper-4471",
                new Shipment.Address("Unit 4, Trafford Park Rd", "Manchester", "M17 1HH", "GB", 53.4655, -2.3204),
                new Shipment.Address("12 Piccadilly", "Manchester", "M1 1AE", "GB", 53.4810, -2.2374),
                Shipment.ServiceLevel.STANDARD,
                carrier,
                Instant.parse("2026-10-22T09:00:00Z"),
                Instant.parse("2026-10-22T11:00:00Z"),
                Instant.parse("2026-10-19T08:00:00Z"));
        shipment.addParcel(new Parcel(4_200, 40, 30, 30));
        shipment.addParcel(new Parcel(1_200, 25, 20, 10));
        return shipment;
    }
}
