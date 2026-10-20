package com.kestrelfreight.shipment.label;

import com.kestrelfreight.shipment.model.Shipment;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import java.time.ZoneId;
import java.time.format.DateTimeFormatter;
import java.util.Locale;

/**
 * Renders a {@link Shipment} as a ZPL II label document.
 */
// @gloss implements .LabelRenderer conf=0.95
@Component
public class LabelRenderer {

    private static final DateTimeFormatter WINDOW_DAY = DateTimeFormatter.ofPattern("EEE d MMM", Locale.UK);
    private static final DateTimeFormatter WINDOW_TIME = DateTimeFormatter.ofPattern("HH:mm", Locale.UK);

    private final ZoneId operatingZone;

    public LabelRenderer(@Value("${kestrel.shipment.operating-zone}") ZoneId operatingZone) {
        this.operatingZone = operatingZone;
    }

    public String render(Shipment shipment) {
        Shipment.Address to = shipment.getDeliveryAddress();
        Shipment.Address from = shipment.getPickupAddress();
        int left = LabelSpec.MARGIN_DOTS;
        int parcels = shipment.getParcels().size();

        StringBuilder zpl = new StringBuilder(512)
                .append("^XA\n")
                .append("^CI28\n")
                .append("^PW").append(LabelSpec.WIDTH_DOTS).append('\n')
                .append("^LL").append(LabelSpec.HEIGHT_DOTS).append('\n')
                .append("^LH0,0\n");

        field(zpl, left, 30, 30, "KESTREL FREIGHT  " + shipment.getServiceLevel());
        field(zpl, left, 70, 22, "Carrier: " + shipment.getCarrier().getCode());
        zpl.append("^FO").append(left).append(",110^GB").append(LabelSpec.WIDTH_DOTS - 2 * left).append(",3,3^FS\n");

        field(zpl, left, 130, 22, "FROM: " + from.line1() + ", " + from.city() + " " + from.postcode());
        field(zpl, left, 190, 28, "TO:");
        field(zpl, left, 230, 40, to.line1());
        field(zpl, left, 280, 40, to.city());
        field(zpl, left, 330, 60, to.postcode() + "  " + to.country());

        var start = shipment.getWindowStart().atZone(operatingZone);
        var end = shipment.getWindowEnd().atZone(operatingZone);
        field(zpl, left, 430, 30, "Deliver %s %s-%s".formatted(
                WINDOW_DAY.format(start), WINDOW_TIME.format(start), WINDOW_TIME.format(end)));
        field(zpl, left, 480, 24, String.format(Locale.ROOT, "Parcels: %d   Weight: %.1f kg",
                parcels, shipment.totalWeightGrams() / 1000.0));

        zpl.append(LabelSpec.code128(shipment.getTrackingNumber(), left + 40, 860)).append('\n');
        zpl.append("^XZ\n");
        return zpl.toString();
    }

    private static void field(StringBuilder zpl, int x, int y, int height, String text) {
        zpl.append("^FO").append(x).append(',').append(y)
                .append("^A0N,").append(height).append(',').append(height)
                .append("^FD").append(escape(text)).append("^FS\n");
    }

    private static String escape(String text) {
        return text.replace("^", "").replace("~", "");
    }
}
