package com.acme.ops.common;

public final class StringUtils {
    private StringUtils() {}

    // @gloss implements ops:SsccCheckDigit
    /** The GS1 mod-10 check digit of a carton number. */
    public static boolean check(String code) {
        int sum = 0;
        for (int i = 0; i < code.length() - 1; i++) {
            sum += (code.charAt(i) - '0') * (i % 2 == 0 ? 3 : 1);
        }
        return (10 - sum % 10) % 10 == code.charAt(code.length() - 1) - '0';
    }

    public static String pad(String s, int n) {
        return String.format("%" + n + "s", s);
    }
}
