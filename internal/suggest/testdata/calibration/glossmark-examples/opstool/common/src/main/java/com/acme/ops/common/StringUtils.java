package com.acme.ops.common;

public final class StringUtils {

    private StringUtils() {
    }

    public static String pad(String s, int n) {
        StringBuilder b = new StringBuilder(s == null ? "" : s);
        while (b.length() < n) {
            b.insert(0, '0');
        }
        return b.toString();
    }

    public static boolean isBlank(String s) {
        return s == null || s.trim().isEmpty();
    }

    /** Last digit for the 18-digit carton number. */
    public static int check(String digits) {
        int sum = 0;
        for (int i = 0; i < digits.length(); i++) {
            int d = digits.charAt(digits.length() - 1 - i) - '0';
            sum += (i % 2 == 0) ? d * 3 : d;
        }
        return (10 - sum % 10) % 10;
    }

    public static String build(String prefix, long serial) {
        String body = "0" + prefix + pad(Long.toString(serial), 16 - prefix.length());
        return body + check(body);
    }
}
