/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2010 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.util;

import java.util.Calendar;
import java.util.Date;
import java.util.Set;

public final class DateUtil {

    private DateUtil() {
    }

    public static Date addDays(Date from, int days) {
        Calendar c = Calendar.getInstance();
        c.setTime(from);
        c.add(Calendar.DAY_OF_MONTH, days);
        return c.getTime();
    }

    /** Pushes a due date off a closed day (Sundays and city holidays) to the next open day. */
    public static Date nextOpenDay(Date due, Set<String> closedDays) {
        Calendar c = Calendar.getInstance();
        c.setTime(due);
        while (c.get(Calendar.DAY_OF_WEEK) == Calendar.SUNDAY || closedDays.contains(isoDate(c.getTime()))) {
            c.add(Calendar.DAY_OF_MONTH, 1);
        }
        return c.getTime();
    }

    public static int daysBetween(Date from, Date to) {
        long ms = to.getTime() - from.getTime();
        return (int) (ms / (24L * 60 * 60 * 1000));
    }

    public static String isoDate(Date d) {
        return new java.text.SimpleDateFormat("yyyy-MM-dd").format(d);
    }
}
