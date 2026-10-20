<!-- @gloss documents .PickupPlanner conf=0.9 -->
# Building a day's pickup plan

The plan for a date is built once, normally by the nightly scheduler calling `POST /routes/plan?date=YYYY-MM-DD` for the next day. `PickupPlanner.planDay` (`src/planning/pickupPlanner.ts`) runs the steps below. Dates and shift times are depot local time (`DEPOT_TIMEZONE`, Europe/London).

## 1. Load the queue and the drivers

Every `PickupRequest` with `pickupDate = date` and status `QUEUED` is loaded, together with every active driver who has a vehicle. Standard bookings are queued for the day after they are booked; express bookings made before the cut-off are queued for the same day (see `express-pickup.md`).

If any route already exists for the date the planner stops with `409 PLAN_EXISTS`. Re-planning a day is an operator action: routes are deleted in the ops console first, which returns their requests to the queue.

## 2. Match requests to drivers

Each request first gets a deadline. A pickup must reach the depot early enough for linehaul to the delivery depot before the shipment's delivery window opens. The deadline is `deliveryWindow.start - LINEHAUL_BUFFER_MINUTES` (240 by default). The deadline drives both driver matching and stop ordering.

`matchDrivers` (`src/planning/driverMatcher.ts`) takes requests in priority order (express first, then earliest deadline, then heaviest) and gives each to one driver who:

- covers the request's **zone** (the postcode area of the pickup address, e.g. `EC`);
- is on **shift** early enough: shift start plus `MIN_LEAD_MINUTES` is no later than the deadline;
- still has **capacity** in their vehicle for both weight (`payloadGrams`) and parcel count (`maxParcels`).

When several drivers qualify, a driver already working that zone is preferred, which keeps routes compact; ties go to the driver with most capacity left. Requests nobody can take are reported in the plan summary as `NO_DRIVER_FOR_ZONE`, `OUTSIDE_SHIFT` or `CAPACITY_EXHAUSTED` and stay `QUEUED`.

## 3. Optimise each route

`optimizeRoute` (`src/planning/routeOptimizer.ts`) orders one driver's stops. Routes start at the depot at the driver's shift start and end back at the depot.

1. **Nearest neighbour.** Repeatedly drive to the closest remaining stop, unless some stop has less than 45 minutes of slack against its deadline, in which case the most urgent stop goes next.
2. **2-opt.** Reverse every segment of the route and keep the change when fewer stops are late, or as many are late over a shorter distance. A change that makes more stops late is never kept.

Distances are haversine (great-circle) kilometres. Travel time multiplies them by a circuity factor of 1.3 and divides by `AVG_SPEED_KMH`; each stop adds `SERVICE_MINUTES`. Stops that are still late after optimisation are reported in `lateShipmentIds` so the ops team can intervene.

## 4. Persist the plan

In one transaction, each route is written with its stops (`seq` 1..n, and an ETA window of `ETA_WINDOW_MINUTES` centred on the predicted arrival), and the planned requests move from `QUEUED` to `PLANNED`. Either the whole day is stored or nothing is.

## 5. Publish RoutePlanned

After the commit, one `RoutePlanned` event per route is published on `dispatch.routes.v1`, keyed by route id. Tracking uses it to show customers their pickup ETA window. The same event is published again whenever a route changes, for example after an express insertion; consumers replace the route by `routeId`.
