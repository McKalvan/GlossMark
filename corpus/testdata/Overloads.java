package zoo;

public class StringUtils {
    // @gloss implements impl:NameCheck
    public static boolean check(String s) {
        return s != null;
    }

    // @gloss implements impl:CountCheck
    public static boolean check(int n) {
        return n > 0;
    }
}
