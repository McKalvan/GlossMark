// @gloss verifies .PickupPlanner scope=file conf=0.9
import type { PrismaClient } from '@prisma/client';
import { pino } from 'pino';
import { describe, expect, it, vi } from 'vitest';
import type { PlanningSettings } from '../src/config.js';
import type { RoutePlanned } from '../src/events/types.js';
import { PickupPlanner, PlanConflictError, pickupDeadline } from '../src/planning/pickupPlanner.js';

const DATE = '2026-10-20';

const settings: PlanningSettings = {
  depot: { lat: 51.5265, lng: -0.2667 },
  timeZone: 'Europe/London',
  expressCutoff: '11:00',
  avgSpeedKmh: 28,
  serviceMinutes: 6,
  etaWindowMinutes: 30,
  linehaulBufferMinutes: 240,
  minLeadMinutes: 20,
};

interface RequestRow {
  id: string;
  shipmentId: string;
  zone: string;
  lat: number;
  lng: number;
  serviceLevel: 'STANDARD' | 'EXPRESS';
  parcelCount: number;
  totalWeightGrams: number;
  deliveryWindowStart: Date;
}

const request = (id: string, zone: string, lat: number, lng: number): RequestRow => ({
  id,
  shipmentId: `shp-${id}`,
  zone,
  lat,
  lng,
  serviceLevel: 'STANDARD',
  parcelCount: 2,
  totalWeightGrams: 4_000,
  deliveryWindowStart: new Date('2026-10-21T08:00:00Z'),
});

const requests: RequestRow[] = [
  request('ec-1', 'EC', 51.5203, -0.0985),
  request('ec-2', 'EC', 51.5155, -0.0922),
  request('ec-3', 'EC', 51.5246, -0.1091),
  request('n-1', 'N', 51.5629, -0.1062),
  request('n-2', 'N', 51.5712, -0.0963),
  request('se-1', 'SE', 51.4826, -0.0077),
];

const drivers = [
  {
    id: 'driver-ec',
    zones: ['EC', 'WC'],
    shiftStart: '08:00',
    shiftEnd: '16:00',
    vehicle: { payloadGrams: 800_000, maxParcels: 150 },
  },
  {
    id: 'driver-n',
    zones: ['N'],
    shiftStart: '07:30',
    shiftEnd: '15:30',
    vehicle: { payloadGrams: 800_000, maxParcels: 150 },
  },
];

interface StopCreate {
  pickupRequestId: string;
  seq: number;
  etaFrom: Date;
  etaTo: Date;
}

interface RouteCreateArgs {
  data: { driverId: string; date: Date; distanceKm: number; stops: { create: StopCreate[] } };
}

function setup(existingRoutes = 0) {
  const byId = new Map(requests.map((r) => [r.id, r]));
  const tx = {
    route: {
      create: vi.fn(async ({ data }: RouteCreateArgs) => ({
        id: `route-${data.driverId}`,
        driverId: data.driverId,
        date: data.date,
        distanceKm: data.distanceKm,
        stops: data.stops.create.map((stop) => ({ ...stop, pickupRequest: byId.get(stop.pickupRequestId)! })),
      })),
    },
    pickupRequest: { updateMany: vi.fn().mockResolvedValue({ count: 0 }) },
  };
  const prisma = {
    route: { count: vi.fn().mockResolvedValue(existingRoutes) },
    pickupRequest: { findMany: vi.fn().mockResolvedValue(requests) },
    driver: { findMany: vi.fn().mockResolvedValue(drivers) },
    $transaction: vi.fn(async (fn: (client: typeof tx) => Promise<unknown>) => fn(tx)),
  } as unknown as PrismaClient;
  const publisher = { publishRoutePlanned: vi.fn<(event: RoutePlanned) => Promise<void>>().mockResolvedValue() };
  const planner = new PickupPlanner(prisma, publisher, settings, pino({ level: 'silent' }));
  return { tx, publisher, planner };
}

describe('PickupPlanner.planDay', () => {
  it('builds one route per matched driver and publishes RoutePlanned for each', async () => {
    const { tx, publisher, planner } = setup();

    const summary = await planner.planDay(DATE);

    expect(summary.routes.map((r) => r.driverId).sort()).toEqual(['driver-ec', 'driver-n']);
    expect(summary.plannedRequests).toBe(5);
    expect(summary.unassigned).toEqual([{ shipmentId: 'shp-se-1', reason: 'NO_DRIVER_FOR_ZONE' }]);
    expect(summary.lateShipmentIds).toEqual([]);

    expect(tx.pickupRequest.updateMany).toHaveBeenCalledWith({
      where: { id: { in: expect.arrayContaining(['ec-1', 'ec-2', 'ec-3', 'n-1', 'n-2']) } },
      data: { status: 'PLANNED' },
    });

    expect(publisher.publishRoutePlanned).toHaveBeenCalledTimes(2);
    const ecEvent = publisher.publishRoutePlanned.mock.calls
      .map(([event]) => event)
      .find((event) => event.driverId === 'driver-ec');
    expect(ecEvent?.date).toBe(DATE);
    expect(ecEvent?.stops.map((s) => s.seq)).toEqual([1, 2, 3]);
    expect(new Set(ecEvent?.stops.map((s) => s.shipmentId))).toEqual(
      new Set(['shp-ec-1', 'shp-ec-2', 'shp-ec-3']),
    );
  });

  it('starts each route at the driver shift start in depot local time', async () => {
    const { publisher, planner } = setup();

    await planner.planDay(DATE);

    const nEvent = publisher.publishRoutePlanned.mock.calls
      .map(([event]) => event)
      .find((event) => event.driverId === 'driver-n');
    // 07:30 BST is 06:30Z; the first ETA window opens after departure minus half the window.
    expect(Date.parse(nEvent!.stops[0]!.etaFrom)).toBeGreaterThan(Date.parse('2026-10-20T06:15:00Z'));
  });

  it('refuses to plan a date that already has routes', async () => {
    const { planner, publisher } = setup(3);

    await expect(planner.planDay(DATE)).rejects.toBeInstanceOf(PlanConflictError);
    expect(publisher.publishRoutePlanned).not.toHaveBeenCalled();
  });
});

describe('pickupDeadline', () => {
  it('leaves the linehaul buffer before the delivery window opens', () => {
    expect(pickupDeadline(new Date('2026-10-21T08:00:00Z'), 240).toISOString()).toBe(
      '2026-10-21T04:00:00.000Z',
    );
  });
});
