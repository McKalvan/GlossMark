package lib;

public class StringUtils {
    // @gloss implements circ:NameCheck
    public static boolean check(String s) {
        return s != null;
    }

    // @gloss implements circ:CountCheck
    public static boolean check(int n) {
        return n > 0;
    }
}
