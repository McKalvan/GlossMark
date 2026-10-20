<!-- @gloss documents .GeofenceArrival conf=0.9 -->
# Geofencing

A **geofence arrival** happens when a driver's GPS ping places them within **75 m** of a shipment's delivery address. The radius is `ARRIVAL_RADIUS_M` in `src/delivery/geofence.rs`.

## Where the delivery address comes from

Tracking never sees the booking. The delivery address coordinates come from the planned stop for the shipment in the latest `RoutePlanned` on `dispatch.routes.v1` (`planned_stops.lat`, `planned_stops.lng`). If dispatch re-plans a route, the newest plan wins.

## The check

For every accepted ping (`src/ingest/ping_ingestor.rs`):

1. Find the driver's shipments that are `IN_TRANSIT` and their latest planned stop.
2. Compute the great-circle distance with the haversine formula (mean Earth radius 6 371 008.8 m).
3. If the distance is at most 75 m, mark the shipment `ARRIVED` and record `arrived_at` as the ping's `recorded_at`.

The boundary is inclusive: exactly 75 m counts as arrived.

## Accuracy

Devices report a horizontal accuracy with each ping.

| Accuracy | Effect |
| --- | --- |
| over 500 m | ping rejected, not stored |
| over 50 m | ping stored and updates last position, but cannot prove arrival |
| 50 m or better, or not reported | eligible for arrival |

## What arrival is not

Arrival is not delivery. It moves the shipment to `ARRIVED` and makes a geofence **proof of delivery** possible; the shipment only becomes `DELIVERED` when a `DELIVERED` scan is recorded. See [delivery-confirmation.md](delivery-confirmation.md).
