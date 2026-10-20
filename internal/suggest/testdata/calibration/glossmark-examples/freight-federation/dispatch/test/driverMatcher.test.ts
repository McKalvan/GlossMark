// @gloss verifies .DriverMatcher scope=file conf=0.9
import { describe, expect, it } from 'vitest';
import {
  matchDrivers,
  type DriverCandidate,
  type MatchableRequest,
} from '../src/planning/driverMatcher.js';

const at = (hhmm: string): Date => new Date(`2026-10-20T${hhmm}:00Z`);

const driver = (overrides: Partial<DriverCandidate> & { driverId: string }): DriverCandidate => ({
  zones: ['EC'],
  shiftStart: at('07:00'),
  shiftEnd: at('15:00'),
  capacityGrams: 500_000,
  capacityParcels: 120,
  ...overrides,
});

const request = (overrides: Partial<MatchableRequest> & { id: string }): MatchableRequest => ({
  shipmentId: `shp-${overrides.id}`,
  zone: 'EC',
  weightGrams: 2_000,
  parcelCount: 1,
  deadline: null,
  express: false,
  ...overrides,
});

const options = { minLeadMinutes: 20 };

describe('matchDrivers', () => {
  it('assigns requests only to drivers covering their zone', () => {
    const result = matchDrivers(
      [request({ id: 'r1', zone: 'EC' }), request({ id: 'r2', zone: 'N' }), request({ id: 'r3', zone: 'SE' })],
      [driver({ driverId: 'd-ec', zones: ['EC'] }), driver({ driverId: 'd-n', zones: ['N', 'NW'] })],
      options,
    );

    expect(result.assignments.get('d-ec')?.map((r) => r.id)).toEqual(['r1']);
    expect(result.assignments.get('d-n')?.map((r) => r.id)).toEqual(['r2']);
    expect(result.unassigned).toEqual([
      { request: expect.objectContaining({ id: 'r3' }), reason: 'NO_DRIVER_FOR_ZONE' },
    ]);
  });

  it('respects vehicle payload and gives express requests priority', () => {
    const result = matchDrivers(
      [request({ id: 'standard', weightGrams: 6_000 }), request({ id: 'express', weightGrams: 6_000, express: true })],
      [driver({ driverId: 'd1', capacityGrams: 10_000 })],
      options,
    );

    expect(result.assignments.get('d1')?.map((r) => r.id)).toEqual(['express']);
    expect(result.unassigned).toEqual([
      { request: expect.objectContaining({ id: 'standard' }), reason: 'CAPACITY_EXHAUSTED' },
    ]);
  });

  it('respects the parcel limit', () => {
    const result = matchDrivers(
      [request({ id: 'r1', parcelCount: 3 }), request({ id: 'r2', parcelCount: 3 })],
      [driver({ driverId: 'd1', capacityParcels: 4 })],
      options,
    );

    expect(result.assignments.get('d1')).toHaveLength(1);
    expect(result.unassigned[0]?.reason).toBe('CAPACITY_EXHAUSTED');
  });

  it('only uses drivers whose shift starts early enough for the deadline', () => {
    const result = matchDrivers(
      [request({ id: 'early', deadline: at('08:30') })],
      [driver({ driverId: 'late-shift', shiftStart: at('09:00') }), driver({ driverId: 'early-shift' })],
      options,
    );

    expect(result.assignments.get('early-shift')?.map((r) => r.id)).toEqual(['early']);
    expect(result.assignments.has('late-shift')).toBe(false);
  });

  it('reports OUTSIDE_SHIFT when no driver in the zone can make the deadline', () => {
    const result = matchDrivers(
      [request({ id: 'r1', deadline: at('07:10') })],
      [driver({ driverId: 'd1' })],
      options,
    );

    expect(result.unassigned[0]?.reason).toBe('OUTSIDE_SHIFT');
  });

  it('keeps a zone on one driver while capacity allows', () => {
    const result = matchDrivers(
      [request({ id: 'r1' }), request({ id: 'r2' }), request({ id: 'r3' })],
      [driver({ driverId: 'd1' }), driver({ driverId: 'd2' })],
      options,
    );

    expect(result.assignments.get('d1')).toHaveLength(3);
    expect(result.assignments.has('d2')).toBe(false);
  });
});
