<!-- @gloss documents .ExpressPickup conf=0.9 -->
# Express pickups

An ExpressPickup is a same-day collection. It is offered only for `EXPRESS` shipments booked before **11:00 depot local time** (`EXPRESS_CUTOFF`). The logic lives in `src/planning/expressPickup.ts`.

## Cut-off

`isWithinExpressCutoff` compares the shipment's `bookedAt` in depot local time with the cut-off: the booking must be on the pickup day and strictly before 11:00. A shipment booked at 11:00:00 is not eligible. Daylight saving changes are handled by converting through the IANA time zone, not by a fixed offset.

When the `ShipmentBooked` event arrives, the consumer already queues an eligible express shipment for the same day; a late one is queued for the next day like a standard shipment.

## Requesting a same-day pickup

`POST /express-pickups` with `{ "shipmentId": "..." }`. The responses are:

| Status | Outcome | Meaning |
| --- | --- | --- |
| 201 | `INSERTED` | Added to a live route; the response carries the route, position and ETA window |
| 202 | `QUEUED_FOR_PLANNING` | Today's plan is not built yet; the planner will include it, express first |
| 422 | `REJECTED_CUTOFF` | Booked at or after the cut-off |
| 409 | `NO_ROUTE_AVAILABLE` | No live route can take it without making a stop late |
| 409 | `NOT_QUEUED` | Already planned, picked up or cancelled |
| 404 | `NOT_FOUND` | No pickup request for that shipment (the booking event has not arrived yet) |

## Choosing the route

Candidate routes are today's routes that are `PLANNED` or `IN_PROGRESS` and whose driver covers the pickup's zone and still has vehicle capacity for its weight and parcels.

## Cheapest insertion

For each candidate route, only the open part is considered: stops the driver has already confirmed cannot move. The open part starts at the last confirmed stop (or the depot) at the later of now and the driver's shift start, and ends back at the depot.

`cheapestInsertion` tries the new stop in every gap of the open part and rejects any position that makes the new stop, or any stop that is currently on time, miss its deadline. Of the remaining positions it picks the one that adds the least distance. Across routes, the route with the smallest added distance wins.

## Persisting and publishing

In one transaction the later stops move down one position, the new stop is created at its `seq`, the ETA windows of the open stops are refreshed, the request becomes `PLANNED` for today, and the route distance is updated. After the commit the whole route is republished as `RoutePlanned` on `dispatch.routes.v1`, so tracking shows the new ETA windows to every affected shipper.
