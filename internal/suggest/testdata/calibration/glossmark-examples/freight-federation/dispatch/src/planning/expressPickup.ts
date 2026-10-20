import type { PickupStatus, PrismaClient } from '@prisma/client';
import type { BaseLogger } from 'pino';
import type { PlanningSettings } from '../config.js';
import { buildRoutePlanned, type EventPublisher } from '../events/publisher.js';
import { localDate, localMinutesOfDay, parseClock, toDbDate, zonedDateTime } from '../time.js';
import type { LatLng } from './geo.js';
import { pickupDeadline } from './pickupPlanner.js';
import {
  etaWindow,
  scheduleRoute,
  type OptimizerOptions,
  type OptimizerStop,
  type RouteSchedule,
} from './routeOptimizer.js';

/**
 * An ExpressPickup is collected the same day it is booked, which is only offered
 * for bookings made strictly before the cut-off (11:00 depot local time by default).
 */
export function isWithinExpressCutoff(
  bookedAt: Date,
  pickupDate: string,
  timeZone: string,
  cutoff: string,
): boolean {
  return (
    localDate(bookedAt, timeZone) === pickupDate &&
    localMinutesOfDay(bookedAt, timeZone) < parseClock(cutoff)
  );
}

export interface InsertionPlan {
  /** Index into the open stops at which the new stop is inserted. */
  position: number;
  addedKm: number;
  schedule: RouteSchedule;
}

/**
 * Cheapest insertion into the open part of a live route: tries every gap between
 * the driver's current position and the depot, rejects positions that would make
 * the new stop, or any stop that is currently on time, late, and returns the
 * position that adds the least distance. Null when no position is feasible.
 */
export function cheapestInsertion(
  openStops: readonly OptimizerStop[],
  candidate: OptimizerStop,
  opts: OptimizerOptions,
): InsertionPlan | null {
  const baseline = scheduleRoute(openStops, opts);
  const alreadyLate = new Set(baseline.lateStopIds);
  let best: InsertionPlan | null = null;

  for (let position = 0; position <= openStops.length; position += 1) {
    const trial = [...openStops.slice(0, position), candidate, ...openStops.slice(position)];
    const schedule = scheduleRoute(trial, opts);
    if (schedule.lateStopIds.some((id) => !alreadyLate.has(id))) continue;
    const addedKm = schedule.distanceKm - baseline.distanceKm;
    if (best === null || addedKm < best.addedKm) best = { position, addedKm, schedule };
  }

  return best;
}

export type ExpressPickupOutcome =
  | {
      outcome: 'INSERTED';
      routeId: string;
      driverId: string;
      seq: number;
      etaFrom: Date;
      etaTo: Date;
      addedKm: number;
    }
  | { outcome: 'QUEUED_FOR_PLANNING'; pickupDate: string }
  | { outcome: 'REJECTED_CUTOFF'; cutoff: string }
  | { outcome: 'NO_ROUTE_AVAILABLE' }
  | { outcome: 'NOT_FOUND' }
  | { outcome: 'NOT_QUEUED'; status: PickupStatus };

// @gloss implements .ExpressPickup conf=0.9
export class ExpressPickupService {
  constructor(
    private readonly prisma: PrismaClient,
    private readonly publisher: Pick<EventPublisher, 'publishRoutePlanned'>,
    private readonly settings: PlanningSettings,
    private readonly logger: BaseLogger,
  ) {}

  async request(shipmentId: string, now: Date = new Date()): Promise<ExpressPickupOutcome> {
    const { timeZone, expressCutoff } = this.settings;
    const request = await this.prisma.pickupRequest.findUnique({ where: { shipmentId } });
    if (!request) return { outcome: 'NOT_FOUND' };
    if (request.status !== 'QUEUED') return { outcome: 'NOT_QUEUED', status: request.status };

    const today = localDate(now, timeZone);
    if (!isWithinExpressCutoff(request.bookedAt, today, timeZone, expressCutoff)) {
      return { outcome: 'REJECTED_CUTOFF', cutoff: expressCutoff };
    }

    const day = toDbDate(today);
    const liveRoutes = await this.prisma.route.findMany({
      where: { date: day, status: { in: ['PLANNED', 'IN_PROGRESS'] } },
      include: {
        driver: { include: { vehicle: true } },
        stops: { include: { pickupRequest: true }, orderBy: { seq: 'asc' } },
      },
    });

    if (liveRoutes.length === 0 && (await this.prisma.route.count({ where: { date: day } })) === 0) {
      // Today's plan has not been built yet: the planner will pick it up as an express request.
      await this.prisma.pickupRequest.update({
        where: { id: request.id },
        data: { pickupDate: day, serviceLevel: 'EXPRESS' },
      });
      return { outcome: 'QUEUED_FOR_PLANNING', pickupDate: today };
    }

    const candidate: OptimizerStop = {
      id: request.id,
      lat: request.lat,
      lng: request.lng,
      deadline: pickupDeadline(request.deliveryWindowStart, this.settings.linehaulBufferMinutes),
    };

    let best: { route: (typeof liveRoutes)[number]; plan: InsertionPlan; completed: number } | null =
      null;

    for (const route of liveRoutes) {
      const vehicle = route.driver.vehicle;
      if (!vehicle || !route.driver.zones.includes(request.zone)) continue;

      const grams = route.stops.reduce((sum, s) => sum + s.pickupRequest.totalWeightGrams, 0);
      const parcels = route.stops.reduce((sum, s) => sum + s.pickupRequest.parcelCount, 0);
      if (
        grams + request.totalWeightGrams > vehicle.payloadGrams ||
        parcels + request.parcelCount > vehicle.maxParcels
      ) {
        continue;
      }

      const completed = route.stops.filter((s) => s.completedAt !== null);
      const open = route.stops.filter((s) => s.completedAt === null);
      const lastCompleted = completed.at(-1)?.pickupRequest;
      const origin: LatLng | undefined = lastCompleted
        ? { lat: lastCompleted.lat, lng: lastCompleted.lng }
        : undefined;
      const shiftStart = zonedDateTime(today, route.driver.shiftStart, timeZone);

      const plan = cheapestInsertion(
        open.map((s) => ({
          id: s.pickupRequestId,
          lat: s.pickupRequest.lat,
          lng: s.pickupRequest.lng,
          deadline: pickupDeadline(s.pickupRequest.deliveryWindowStart, this.settings.linehaulBufferMinutes),
        })),
        candidate,
        {
          depot: this.settings.depot,
          origin,
          departAt: new Date(Math.max(now.getTime(), shiftStart.getTime())),
          avgSpeedKmh: this.settings.avgSpeedKmh,
          serviceMinutes: this.settings.serviceMinutes,
        },
      );

      if (plan && (best === null || plan.addedKm < best.plan.addedKm)) {
        best = { route, plan, completed: completed.length };
      }
    }

    if (!best) {
      this.logger.warn({ shipmentId, zone: request.zone }, 'no live route can take express pickup');
      return { outcome: 'NO_ROUTE_AVAILABLE' };
    }

    const { route, plan, completed } = best;
    const insertSeq = completed + plan.position + 1;
    const etas = new Map(
      plan.schedule.stops.map((s) => [s.id, etaWindow(s.arrivalAt, this.settings.etaWindowMinutes)]),
    );
    const newEta = etas.get(request.id);
    if (!newEta) throw new Error(`insertion plan for route ${route.id} lost the express stop`);

    const updated = await this.prisma.$transaction(async (tx) => {
      // Shift later stops down one position, highest first, to keep (route_id, seq) unique.
      const shifting = route.stops.filter((s) => s.seq >= insertSeq).sort((a, b) => b.seq - a.seq);
      for (const stop of shifting) {
        await tx.routeStop.update({ where: { id: stop.id }, data: { seq: stop.seq + 1 } });
      }
      await tx.routeStop.create({
        data: { routeId: route.id, pickupRequestId: request.id, seq: insertSeq, ...newEta },
      });
      for (const [pickupRequestId, eta] of etas) {
        if (pickupRequestId === request.id) continue;
        await tx.routeStop.update({ where: { pickupRequestId }, data: eta });
      }
      await tx.pickupRequest.update({
        where: { id: request.id },
        data: { status: 'PLANNED', pickupDate: day, serviceLevel: 'EXPRESS' },
      });
      return tx.route.update({
        where: { id: route.id },
        data: { distanceKm: { increment: plan.addedKm } },
        include: { stops: { include: { pickupRequest: true }, orderBy: { seq: 'asc' } } },
      });
    });

    await this.publisher.publishRoutePlanned(buildRoutePlanned(updated));

    this.logger.info(
      { shipmentId, routeId: route.id, driverId: route.driverId, seq: insertSeq, addedKm: plan.addedKm },
      'express pickup inserted into live route',
    );

    return {
      outcome: 'INSERTED',
      routeId: route.id,
      driverId: route.driverId,
      seq: insertSeq,
      etaFrom: newEta.etaFrom,
      etaTo: newEta.etaTo,
      addedKm: plan.addedKm,
    };
  }
}
