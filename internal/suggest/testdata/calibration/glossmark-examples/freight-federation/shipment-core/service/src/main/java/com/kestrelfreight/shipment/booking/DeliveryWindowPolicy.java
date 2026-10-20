package com.kestrelfreight.shipment.booking;

import com.kestrelfreight.shipment.model.Shipment.ServiceLevel;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import java.time.DayOfWeek;
import java.time.Duration;
import java.time.Instant;
import java.time.LocalDate;
import java.time.LocalTime;
import java.time.ZoneId;
import java.time.ZonedDateTime;

/**
 * Decides the delivery window promised to the shipper at booking time.
 */
// @gloss implements .DeliveryWindowPolicy conf=0.95
@Component
public final class DeliveryWindowPolicy {

    public static final Duration WINDOW_LENGTH = Duration.ofHours(2);

    public static final LocalTime EXPRESS_CUTOFF = LocalTime.of(11, 0);
    public static final LocalTime EXPRESS_SAME_DAY_START = LocalTime.of(15, 0);
    public static final LocalTime FIRST_WINDOW_START = LocalTime.of(8, 0);
    public static final LocalTime LAST_WINDOW_START = LocalTime.of(18, 0);
    public static final LocalTime DEFAULT_WINDOW_START = LocalTime.of(10, 0);

    private final ZoneId operatingZone;

    public DeliveryWindowPolicy(@Value("${kestrel.shipment.operating-zone}") ZoneId operatingZone) {
        this.operatingZone = operatingZone;
    }

    public record DeliveryWindow(Instant start, Instant end) {
    }

    public DeliveryWindow promise(LocalDate requestedDate, ServiceLevel serviceLevel, LocalTime preferredStart,
                                  Instant bookedAt) {
        ZonedDateTime booked = bookedAt.atZone(operatingZone);
        LocalDate bookingDate = booked.toLocalDate();
        if (requestedDate.isBefore(bookingDate)) {
            throw new IllegalArgumentException("Requested date " + requestedDate + " is before booking date " + bookingDate);
        }

        if (serviceLevel == ServiceLevel.EXPRESS
                && requestedDate.equals(bookingDate)
                && booked.toLocalTime().isBefore(EXPRESS_CUTOFF)
                && isWorkingDay(bookingDate)) {
            return windowAt(bookingDate, EXPRESS_SAME_DAY_START);
        }

        LocalDate earliest = serviceLevel == ServiceLevel.EXPRESS
                ? nextWorkingDay(bookingDate)
                : nextWorkingDay(nextWorkingDay(bookingDate));
        LocalDate deliveryDate = requestedDate.isBefore(earliest) ? earliest : rollToWorkingDay(requestedDate);
        return windowAt(deliveryDate, clampStart(preferredStart));
    }

    private DeliveryWindow windowAt(LocalDate date, LocalTime start) {
        ZonedDateTime from = date.atTime(start).atZone(operatingZone);
        return new DeliveryWindow(from.toInstant(), from.plus(WINDOW_LENGTH).toInstant());
    }

    private static LocalTime clampStart(LocalTime preferred) {
        if (preferred == null) {
            return DEFAULT_WINDOW_START;
        }
        LocalTime onTheHour = preferred.withMinute(0).withSecond(0).withNano(0);
        if (onTheHour.isBefore(FIRST_WINDOW_START)) {
            return FIRST_WINDOW_START;
        }
        return onTheHour.isAfter(LAST_WINDOW_START) ? LAST_WINDOW_START : onTheHour;
    }

    private static LocalDate nextWorkingDay(LocalDate date) {
        return rollToWorkingDay(date.plusDays(1));
    }

    private static LocalDate rollToWorkingDay(LocalDate date) {
        LocalDate candidate = date;
        while (!isWorkingDay(candidate)) {
            candidate = candidate.plusDays(1);
        }
        return candidate;
    }

    private static boolean isWorkingDay(LocalDate date) {
        DayOfWeek day = date.getDayOfWeek();
        return day != DayOfWeek.SATURDAY && day != DayOfWeek.SUNDAY;
    }
}
