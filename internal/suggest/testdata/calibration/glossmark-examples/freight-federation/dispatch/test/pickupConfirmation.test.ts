// @gloss verifies .PickupConfirmation scope=file conf=0.9
import type { PrismaClient } from '@prisma/client';
import { pino } from 'pino';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  ConfirmationError,
  PickupConfirmationService,
} from '../src/confirm/pickupConfirmation.js';

const DRIVER_ID = '6f1c2a9e-3b7d-4c55-9a0e-1d2f3a4b5c6d';
const ROUTE_ID = 'b0e1f2a3-4c5d-4e6f-8a9b-0c1d2e3f4a5b';
const REQUEST_ID = '0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d';
const confirmedAt = new Date('2026-10-20T09:12:00Z');

function plannedRequest(overrides: Record<string, unknown> = {}) {
  return {
    id: REQUEST_ID,
    shipmentId: 'shp_01J9Z6Q4M2',
    trackingNumber: 'KF12345678903',
    status: 'PLANNED',
    pickedUpAt: null,
    stop: {
      id: 'stop-1',
      route: { id: ROUTE_ID, driverId: DRIVER_ID, status: 'PLANNED' },
    },
    ...overrides,
  };
}

function setup(request: unknown, openStopsAfter = 3) {
  const tx = {
    pickupRequest: {
      findUnique: vi.fn().mockResolvedValue(request),
      update: vi.fn().mockImplementation(async ({ data }: { data: Record<string, unknown> }) => ({
        ...(request as object),
        ...data,
      })),
    },
    routeStop: {
      update: vi.fn().mockResolvedValue({}),
      count: vi.fn().mockResolvedValue(openStopsAfter),
    },
    route: { update: vi.fn().mockResolvedValue({}) },
  };
  const prisma = {
    $transaction: vi.fn(async (fn: (client: typeof tx) => Promise<unknown>) => fn(tx)),
  } as unknown as PrismaClient;
  const publisher = { publishShipmentPickedUp: vi.fn().mockResolvedValue(undefined) };
  const service = new PickupConfirmationService(prisma, publisher, pino({ level: 'silent' }));
  return { tx, publisher, service };
}

const input = {
  driverId: DRIVER_ID,
  trackingNumber: 'KF12345678903',
  lat: 51.5226,
  lng: -0.1571,
  confirmedAt,
};

describe('PickupConfirmationService', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('marks the request picked up and publishes ShipmentPickedUp', async () => {
    const { tx, publisher, service } = setup(plannedRequest());

    const result = await service.confirm(REQUEST_ID, input);

    expect(tx.pickupRequest.update).toHaveBeenCalledWith({
      where: { id: REQUEST_ID },
      data: { status: 'PICKED_UP', pickedUpAt: confirmedAt, pickedUpBy: DRIVER_ID },
    });
    expect(tx.routeStop.update).toHaveBeenCalledWith({
      where: { id: 'stop-1' },
      data: { completedAt: confirmedAt },
    });
    expect(tx.route.update).toHaveBeenCalledWith({
      where: { id: ROUTE_ID },
      data: { status: 'IN_PROGRESS' },
    });
    expect(publisher.publishShipmentPickedUp).toHaveBeenCalledWith({
      shipmentId: 'shp_01J9Z6Q4M2',
      trackingNumber: 'KF12345678903',
      driverId: DRIVER_ID,
      routeId: ROUTE_ID,
      pickedUpAt: '2026-10-20T09:12:00.000Z',
      lat: 51.5226,
      lng: -0.1571,
    });
    expect(result.alreadyConfirmed).toBe(false);
  });

  it('completes the route when the last stop is confirmed', async () => {
    const { tx, service } = setup(plannedRequest(), 0);

    await service.confirm(REQUEST_ID, input);

    expect(tx.route.update).toHaveBeenCalledWith({
      where: { id: ROUTE_ID },
      data: { status: 'COMPLETED' },
    });
  });

  it('treats a repeated confirmation as idempotent and republishes the original time', async () => {
    const original = new Date('2026-10-20T09:05:00Z');
    const { tx, publisher, service } = setup(
      plannedRequest({ status: 'PICKED_UP', pickedUpAt: original }),
    );

    const result = await service.confirm(REQUEST_ID, input);

    expect(result.alreadyConfirmed).toBe(true);
    expect(tx.pickupRequest.update).not.toHaveBeenCalled();
    expect(publisher.publishShipmentPickedUp).toHaveBeenCalledWith(
      expect.objectContaining({ pickedUpAt: original.toISOString() }),
    );
  });

  it('rejects a confirmation from a driver who does not own the route', async () => {
    const { tx, publisher, service } = setup(plannedRequest());

    await expect(
      service.confirm(REQUEST_ID, { ...input, driverId: '11111111-2222-4333-8444-555555555555' }),
    ).rejects.toMatchObject({ code: 'WRONG_DRIVER' });
    expect(tx.pickupRequest.update).not.toHaveBeenCalled();
    expect(publisher.publishShipmentPickedUp).not.toHaveBeenCalled();
  });

  it('rejects a scanned label that does not match the request', async () => {
    const { publisher, service } = setup(plannedRequest());

    await expect(
      service.confirm(REQUEST_ID, { ...input, trackingNumber: 'KF99999999990' }),
    ).rejects.toBeInstanceOf(ConfirmationError);
    expect(publisher.publishShipmentPickedUp).not.toHaveBeenCalled();
  });

  it('rejects requests that are not on a route', async () => {
    const { service } = setup(plannedRequest({ status: 'QUEUED', stop: null }));

    await expect(service.confirm(REQUEST_ID, input)).rejects.toMatchObject({ code: 'NOT_PLANNED' });
  });

  it('reports unknown requests', async () => {
    const { service } = setup(null);

    await expect(service.confirm(REQUEST_ID, input)).rejects.toMatchObject({ code: 'NOT_FOUND' });
  });
});
