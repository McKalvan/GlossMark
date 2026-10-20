import type { PickupRequest, PrismaClient } from '@prisma/client';
import type { BaseLogger } from 'pino';
import type { PlanningSettings } from '../config.js';
import { buildRoutePlanned, type EventPublisher } from '../events/publisher.js';
import { addMinutes, toDbDate, zonedDateTime } from '../time.js';
import {
  matchDrivers,
  type DriverCandidate,
  type MatchableRequest,
  type UnassignedReason,
} from './driverMatcher.js';
import { etaWindow, optimizeRoute, type RouteSchedule } from './routeOptimizer.js';

export class PlanConflictError extends Error {
  constructor(readonly date: string) {
    super(`routes already exist for ${date}`);
    this.name = 'PlanConflictError';
  }
}

export interface PlanSummary {
  date: string;
  routes: Array<{ routeId: string; driverId: string; stops: number; distanceKm: number }>;
  plannedRequests: number;
  unassigned: Array<{ shipmentId: string; reason: UnassignedReason }>;
  lateShipmentIds: string[];
}

/** The latest pickup that still leaves time for linehaul before the delivery window opens. */
export function pickupDeadline(deliveryWindowStart: Date, linehaulBufferMinutes: number): Date {
  return addMinutes(deliveryWindowStart, -linehaulBufferMinutes);
}

function toMatchable(request: PickupRequest, settings: PlanningSettings): MatchableRequest {
  return {
    id: request.id,
    shipmentId: request.shipmentId,
    zone: request.zone,
    weightGrams: request.totalWeightGrams,
    parcelCount: request.parcelCount,
    deadline: pickupDeadline(request.deliveryWindowStart, settings.linehaulBufferMinutes),
    express: request.serviceLevel === 'EXPRESS',
  };
}

const roundKm = (km: number): number => Math.round(km * 1000) / 1000;

interface RouteDraft {
  driverId: string;
  schedule: RouteSchedule;
}

// @gloss implements .PickupPlanner conf=0.95
export class PickupPlanner {
  constructor(
    private readonly prisma: PrismaClient,
    private readonly publisher: Pick<EventPublisher, 'publishRoutePlanned'>,
    private readonly settings: PlanningSettings,
    private readonly logger: BaseLogger,
  ) {}

  /** Builds, stores and publishes every driver's pickup route for `date` (YYYY-MM-DD, depot local). */
  async planDay(date: string): Promise<PlanSummary> {
    const day = toDbDate(date);

    // 1. Load the day's queue and the drivers available to work it.
    const existingRoutes = await this.prisma.route.count({ where: { date: day } });
    if (existingRoutes > 0) throw new PlanConflictError(date);

    const [requests, drivers] = await Promise.all([
      this.prisma.pickupRequest.findMany({ where: { pickupDate: day, status: 'QUEUED' } }),
      this.prisma.driver.findMany({
        where: { active: true, vehicleId: { not: null } },
        include: { vehicle: true },
      }),
    ]);

    const candidates: DriverCandidate[] = drivers.flatMap((driver) =>
      driver.vehicle
        ? [
            {
              driverId: driver.id,
              zones: driver.zones,
              shiftStart: zonedDateTime(date, driver.shiftStart, this.settings.timeZone),
              shiftEnd: zonedDateTime(date, driver.shiftEnd, this.settings.timeZone),
              capacityGrams: driver.vehicle.payloadGrams,
              capacityParcels: driver.vehicle.maxParcels,
            },
          ]
        : [],
    );

    // 2. Match requests to drivers by zone, shift and vehicle capacity.
    const match = matchDrivers(
      requests.map((request) => toMatchable(request, this.settings)),
      candidates,
      { minLeadMinutes: this.settings.minLeadMinutes },
    );

    // 3. Optimise the stop order of each driver's route.
    const requestsById = new Map(requests.map((request) => [request.id, request]));
    const drafts: RouteDraft[] = [];
    for (const [driverId, assigned] of match.assignments) {
      const driver = candidates.find((c) => c.driverId === driverId);
      if (!driver) continue;
      const schedule = optimizeRoute(
        assigned.map((a) => {
          const request = requestsById.get(a.id);
          if (!request) throw new Error(`assigned unknown pickup request ${a.id}`);
          return { id: request.id, lat: request.lat, lng: request.lng, deadline: a.deadline };
        }),
        {
          depot: this.settings.depot,
          departAt: driver.shiftStart,
          avgSpeedKmh: this.settings.avgSpeedKmh,
          serviceMinutes: this.settings.serviceMinutes,
        },
      );
      drafts.push({ driverId, schedule });
    }

    // 4. Persist routes and stops, and move the requests out of the queue, atomically.
    const plannedIds = drafts.flatMap((draft) => draft.schedule.stops.map((stop) => stop.id));
    const routes = await this.prisma.$transaction(
      async (tx) => {
        const created = [];
        for (const draft of drafts) {
          created.push(
            await tx.route.create({
              data: {
                driverId: draft.driverId,
                date: day,
                status: 'PLANNED',
                distanceKm: roundKm(draft.schedule.distanceKm),
                stops: {
                  create: draft.schedule.stops.map((stop) => ({
                    pickupRequestId: stop.id,
                    seq: stop.seq,
                    ...etaWindow(stop.arrivalAt, this.settings.etaWindowMinutes),
                  })),
                },
              },
              include: { stops: { include: { pickupRequest: true }, orderBy: { seq: 'asc' } } },
            }),
          );
        }
        await tx.pickupRequest.updateMany({
          where: { id: { in: plannedIds } },
          data: { status: 'PLANNED' },
        });
        return created;
      },
      { timeout: 30_000 },
    );

    // 5. Publish RoutePlanned for the tracking service, one event per route.
    for (const route of routes) {
      await this.publisher.publishRoutePlanned(buildRoutePlanned(route));
    }

    const lateIds = new Set(drafts.flatMap((draft) => draft.schedule.lateStopIds));
    const summary: PlanSummary = {
      date,
      routes: routes.map((route) => ({
        routeId: route.id,
        driverId: route.driverId,
        stops: route.stops.length,
        distanceKm: route.distanceKm,
      })),
      plannedRequests: plannedIds.length,
      unassigned: match.unassigned.map(({ request, reason }) => ({
        shipmentId: request.shipmentId,
        reason,
      })),
      lateShipmentIds: requests.filter((r) => lateIds.has(r.id)).map((r) => r.shipmentId),
    };

    this.logger.info(
      {
        date,
        routes: summary.routes.length,
        plannedRequests: summary.plannedRequests,
        unassigned: summary.unassigned.length,
        late: summary.lateShipmentIds.length,
      },
      'pickup plan built',
    );
    return summary;
  }
}
