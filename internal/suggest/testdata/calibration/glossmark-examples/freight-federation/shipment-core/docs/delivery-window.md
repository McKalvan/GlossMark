# Delivery window and on-time rate

<!-- @gloss documents shc:DeliveryWindowPolicy conf=0.9 -->
## The promise

At booking, shipment-core promises the shipper a **two-hour delivery window**
(`DeliveryWindowPolicy.WINDOW_LENGTH`). The window is fixed at booking and published in
`ShipmentBooked.deliveryWindow`; it is never moved afterwards.

Rules, evaluated in the operating zone (`kestrel.shipment.operating-zone`, Europe/London):

- **Express pickup, same day**: an `EXPRESS` shipment booked before 11:00 local on a working day for that
  same day gets the 15:00-17:00 window.
- **Express, otherwise**: earliest delivery is the next working day.
- **Standard**: earliest delivery is the second working day after booking.
- A requested date earlier than the earliest date is moved forward; weekend dates roll to Monday.
- The window starts at the shipper's preferred hour, clamped to 08:00-18:00, default 10:00.

<!-- @gloss documents frt:OnTimeDeliveryRate conf=0.9 -->
## Measuring on-time

When `ShipmentDelivered` arrives, the delivery is **on time** if `deliveredAt` is within
`[window_start, window_end]` inclusive. The result is stored once per shipment in `delivery_outcomes`.

The **on-time delivery rate** is

```
on-time deliveries / all deliveries, over the last 7 days (rolling)
```

Target: **95%** (`OnTimeRateCalculator.TARGET = 0.95`). With no deliveries in the period the rate is
reported as 0 and the target is not met.

Exposed as:

- `GET /metrics/on-time-rate` (JSON with `delivered`, `onTime`, `rate`, `target`)
- Micrometer gauge `shipment.delivery.on_time_rate` (and `shipment.delivery.on_time_rate.target`) on
  `/actuator/prometheus`, refreshed every five minutes
