# Event catalogue

shipment-core publishes one event and consumes two. All messages are JSON with a CloudEvents-style
envelope:

```json
{ "type": "ShipmentBooked", "id": "<uuid>", "time": "2026-10-19T08:00:00Z", "data": { ... } }
```

The Kafka message key is always the `shipmentId`, so every event for one shipment lands on the same
partition and is consumed in order.

<!-- @gloss documents frt:ShipmentBooked conf=0.9 -->
## ShipmentBooked

- Topic: `shipment.booked.v1`
- Direction: produced by shipment-core, through the transactional outbox (`outbox_events`, relayed by `OutboxRelay`)
- Consumers: dispatch

```json
{
  "shipmentId": "0f8e2c3a-5b1d-4c7e-9a64-2d1f0b7e8c11",
  "trackingNumber": "KF12345678903",
  "shipperId": "shipper-4471",
  "pickupAddress":   { "line1": "Unit 4, Trafford Park Rd", "city": "Manchester", "postcode": "M17 1HH", "country": "GB", "lat": 53.4655, "lng": -2.3204 },
  "deliveryAddress": { "line1": "12 Piccadilly", "city": "Manchester", "postcode": "M1 1AE", "country": "GB", "lat": 53.4810, "lng": -2.2374 },
  "parcels": [ { "parcelId": "6a0c...", "weightGrams": 4200, "lengthCm": 40, "widthCm": 30, "heightCm": 30 } ],
  "serviceLevel": "STANDARD",
  "carrierCode": "NLX",
  "deliveryWindow": { "start": "2026-10-22T09:00:00Z", "end": "2026-10-22T11:00:00Z" },
  "bookedAt": "2026-10-19T08:00:00Z"
}
```

`serviceLevel` is `STANDARD` or `EXPRESS`. The row is written in the same database transaction as the
shipment, so a booking is never visible without its event, and the event is never sent for a booking
that rolled back. Delivery is at-least-once; consumers must de-duplicate on the envelope `id`.

<!-- @gloss documents frt:ShipmentPickedUp conf=0.9 -->
## ShipmentPickedUp

- Topic: `shipment.picked-up.v1`
- Direction: consumed by shipment-core (`ShipmentStatusListener`); produced by dispatch
- Effect: status `BOOKED` -> `PICKED_UP`, `picked_up_at` set

```json
{ "shipmentId": "0f8e2c3a-...", "trackingNumber": "KF12345678903", "driverId": "drv-208", "routeId": "rt-2026-10-22-07",
  "pickedUpAt": "2026-10-21T15:12:44Z", "lat": 53.4655, "lng": -2.3204 }
```

<!-- @gloss documents frt:ShipmentDelivered conf=0.9 -->
## ShipmentDelivered

- Topic: `shipment.delivered.v1`
- Direction: consumed by shipment-core (`ShipmentStatusListener`); produced by tracking
- Effect: status -> `DELIVERED`, one row in `delivery_outcomes` recording whether delivery fell inside the
  promised window (see [delivery-window.md](delivery-window.md))

```json
{ "shipmentId": "0f8e2c3a-...", "trackingNumber": "KF12345678903", "deliveredAt": "2026-10-22T10:41:03Z",
  "lat": 53.4810, "lng": -2.2374, "proof": { "kind": "SIGNATURE", "ref": "pod/2026/10/22/KF12345678903.png" },
  "withinWindow": true }
```

`proof.kind` is `SIGNATURE`, `PHOTO` or `GEOFENCE`. shipment-core recomputes `withinWindow` from its own
promise and logs any disagreement with the value tracking sent.
