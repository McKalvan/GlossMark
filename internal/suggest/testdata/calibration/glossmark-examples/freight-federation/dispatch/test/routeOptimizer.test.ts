// @gloss verifies .RouteOptimizer scope=file conf=0.9
import { describe, expect, it } from 'vitest';
import {
  etaWindow,
  optimizeRoute,
  scheduleRoute,
  type OptimizerOptions,
  type OptimizerStop,
} from '../src/planning/routeOptimizer.js';

const departAt = new Date('2026-10-20T07:00:00Z');

const options: OptimizerOptions = {
  depot: { lat: 51.5, lng: -0.25 },
  departAt,
  avgSpeedKmh: 30,
  serviceMinutes: 5,
};

const stop = (id: string, lng: number, deadline: Date | null = null): OptimizerStop => ({
  id,
  lat: 51.5,
  lng,
  deadline,
});

describe('optimizeRoute', () => {
  it('returns an empty route for no stops', () => {
    expect(optimizeRoute([], options)).toEqual({ stops: [], distanceKm: 0, lateStopIds: [] });
  });

  it('visits collinear stops outward from the depot', () => {
    const route = optimizeRoute(
      [stop('c', -0.1), stop('a', -0.2), stop('d', -0.05), stop('b', -0.15)],
      options,
    );

    expect(route.stops.map((s) => s.id)).toEqual(['a', 'b', 'c', 'd']);
    expect(route.stops.map((s) => s.seq)).toEqual([1, 2, 3, 4]);
    expect(route.lateStopIds).toEqual([]);
  });

  it('visits a stop with a tight deadline first even when it is furthest away', () => {
    const tight = new Date(departAt.getTime() + 45 * 60_000);
    const route = optimizeRoute(
      [stop('near', -0.2), stop('mid', -0.15), stop('far', -0.05, tight)],
      options,
    );

    expect(route.stops[0]?.id).toBe('far');
    expect(route.lateStopIds).toEqual([]);
  });

  it('never produces a longer route than the order it was given', () => {
    const scattered: OptimizerStop[] = [
      { id: 's1', lat: 51.53, lng: -0.12, deadline: null },
      { id: 's2', lat: 51.49, lng: -0.21, deadline: null },
      { id: 's3', lat: 51.55, lng: -0.19, deadline: null },
      { id: 's4', lat: 51.47, lng: -0.1, deadline: null },
      { id: 's5', lat: 51.51, lng: -0.16, deadline: null },
      { id: 's6', lat: 51.56, lng: -0.08, deadline: null },
    ];

    const optimized = optimizeRoute(scattered, options);
    const asGiven = scheduleRoute(scattered, options);

    expect(optimized.distanceKm).toBeLessThanOrEqual(asGiven.distanceKm);
    expect(new Set(optimized.stops.map((s) => s.id))).toEqual(new Set(scattered.map((s) => s.id)));
  });
});

describe('scheduleRoute', () => {
  it('keeps the given order and schedules arrivals with service time between stops', () => {
    const schedule = scheduleRoute([stop('b', -0.15), stop('a', -0.2)], options);

    expect(schedule.stops.map((s) => s.id)).toEqual(['b', 'a']);
    const first = schedule.stops[0]!.arrivalAt.getTime();
    const second = schedule.stops[1]!.arrivalAt.getTime();
    expect(first).toBeGreaterThan(departAt.getTime());
    expect(second - first).toBeGreaterThan(5 * 60_000);
  });

  it('flags stops reached after their deadline', () => {
    const impossible = new Date(departAt.getTime() + 5 * 60_000);
    const schedule = scheduleRoute([stop('far', -0.05, impossible)], options);

    expect(schedule.lateStopIds).toEqual(['far']);
  });
});

describe('etaWindow', () => {
  it('centres the window on the arrival', () => {
    const { etaFrom, etaTo } = etaWindow(new Date('2026-10-20T10:00:00Z'), 30);
    expect(etaFrom.toISOString()).toBe('2026-10-20T09:45:00.000Z');
    expect(etaTo.toISOString()).toBe('2026-10-20T10:15:00.000Z');
  });
});
