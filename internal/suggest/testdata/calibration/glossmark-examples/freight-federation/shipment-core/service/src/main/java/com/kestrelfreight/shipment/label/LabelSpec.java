package com.kestrelfreight.shipment.label;

import java.util.regex.Pattern;

/**
 * Physical and encoding constants for Kestrel shipping labels.
 */
// @gloss implements .LabelSpec conf=0.95
public final class LabelSpec {

    public static final String FORMAT = "ZPL II, 4x6 in, 203 dpi";

    public static final int DPI = 203;
    public static final double WIDTH_INCHES = 4.0;
    public static final double HEIGHT_INCHES = 6.0;
    public static final int WIDTH_DOTS = (int) Math.round(WIDTH_INCHES * DPI);
    public static final int HEIGHT_DOTS = (int) Math.round(HEIGHT_INCHES * DPI);
    public static final int MARGIN_DOTS = 30;

    public static final int BARCODE_MODULE_WIDTH = 3;
    public static final int BARCODE_HEIGHT_DOTS = 160;

    private static final Pattern TRACKING_NUMBER = Pattern.compile("KF\\d{11}");

    private LabelSpec() {
    }

    /**
     * Code 128 barcode of the tracking number at the given origin. Uses subset B for the "KF" prefix
     * and lets the printer switch to subset C for the digits (ZPL auto mode).
     */
    public static String code128(String trackingNumber, int x, int y) {
        if (!TRACKING_NUMBER.matcher(trackingNumber).matches()) {
            throw new IllegalArgumentException("Not a Kestrel tracking number: " + trackingNumber);
        }
        return "^FO%d,%d^BY%d^BCN,%d,Y,N,N,A^FD%s^FS".formatted(
                x, y, BARCODE_MODULE_WIDTH, BARCODE_HEIGHT_DOTS, trackingNumber);
    }
}
