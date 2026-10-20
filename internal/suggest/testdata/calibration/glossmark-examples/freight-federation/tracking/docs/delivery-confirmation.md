<!-- @gloss documents .DeliveryConfirmation conf=0.9 -->
# Delivery confirmation

A shipment is delivered when the driver's handheld sends a `DELIVERED` scan to `POST /scans`. `src/delivery/confirmation.rs` turns that scan into a proof of delivery, decides whether it was on time, and publishes `ShipmentDelivered`.

## Proof of delivery

| Kind | Source | Extra rule |
| --- | --- | --- |
| `SIGNATURE` | signature captured on the handheld, `ref` is the stored image key | none |
| `PHOTO` | photo of the parcel at the door, `ref` is the stored image key | none |
| `GEOFENCE` | used when the scan carries no signature or photo | handheld position must be within 75 m of the delivery address |

The position used is the scan's own coordinates, or the shipment's last known ping when the scanner has no GPS. A `GEOFENCE` proof outside the radius is rejected with `422 confirmation_rejected`. Every proof needs a non-empty `ref`.

<!-- @gloss documents shc:DeliveryWindowPolicy conf=0.9 -->
## Delivery window

The delivery window is **2 hours** (`DELIVERY_WINDOW` in `src/delivery/window.rs`), starting at the promised start. Tracking uses the planned stop's `etaFrom` as the promised start.

- Delivered at 1 h 59 min after the start: on time.
- Delivered at exactly 2 h: on time (inclusive).
- Delivered at 2 h 01 min: late.
- Delivered before the start: on time.

The result is stored as `deliveries.within_window` and sent as `withinWindow`. shipment-core uses it for the OnTimeDeliveryRate.

## Publishing

1. In one transaction: insert the `deliveries` row and set the shipment to `DELIVERED`.
2. After commit, publish `ShipmentDelivered` to `shipment.delivered.v1`, keyed by `shipmentId`.
3. On success, set `deliveries.published_at`.

If Kafka is unavailable the delivery is still recorded. A sweep in `main.rs` runs every 60 s and republishes rows with `published_at IS NULL` that are older than 30 s. Consumers must therefore treat `ShipmentDelivered` as at-least-once.

A second `DELIVERED` scan for an already delivered shipment returns `409`.
