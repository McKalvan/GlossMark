// @gloss verifies .ExpressPickup scope=file conf=0.9
import { describe, expect, it } from 'vitest';
import { cheapestInsertion, isWithinExpressCutoff } from '../src/planning/expressPickup.js';
import type { OptimizerOptions, OptimizerStop } from '../src/planning/routeOptimizer.js';

const LONDON = 'Europe/London';

describe('isWithinExpressCutoff', () => {
  it('accepts bookings made before 11:00 local time in summer time', () => {
    // 09:59Z is 10:59 BST
    expect(isWithinExpressCutoff(new Date('2026-10-19T09:59:00Z'), '2026-10-19', LONDON, '11:00')).toBe(true);
  });

  it('rejects bookings made at or after 11:00 local time', () => {
    expect(isWithinExpressCutoff(new Date('2026-10-19T10:00:00Z'), '2026-10-19', LONDON, '11:00')).toBe(false);
    expect(isWithinExpressCutoff(new Date('2026-10-19T13:30:00Z'), '2026-10-19', LONDON, '11:00')).toBe(false);
  });

  it('uses GMT after the clocks go back', () => {
    expect(isWithinExpressCutoff(new Date('2026-11-02T10:30:00Z'), '2026-11-02', LONDON, '11:00')).toBe(true);
  });

  it('rejects bookings made on a previous day', () => {
    expect(isWithinExpressCutoff(new Date('2026-10-18T08:00:00Z'), '2026-10-19', LONDON, '11:00')).toBe(false);
  });
});

describe('cheapestInsertion', () => {
  const departAt = new Date('2026-10-19T10:30:00Z');
  const options: OptimizerOptions = {
    depot: { lat: 51.5, lng: -0.25 },
    departAt,
    avgSpeedKmh: 30,
    serviceMinutes: 5,
  };
  const stop = (id: string, lng: number, deadline: Date | null = null, lat = 51.5): OptimizerStop => ({
    id,
    lat,
    lng,
    deadline,
  });
  const minutesAfterDepart = (m: number): Date => new Date(departAt.getTime() + m * 60_000);

  it('inserts the stop into the gap that adds least distance', () => {
    const plan = cheapestInsertion(
      [stop('a', -0.2, null, 51.52), stop('b', -0.1, null, 51.52)],
      stop('express', -0.15, null, 51.52),
      options,
    );

    expect(plan?.position).toBe(1);
    expect(plan?.schedule.stops.map((s) => s.id)).toEqual(['a', 'express', 'b']);
    expect(plan?.addedKm).toBeLessThan(0.05);
  });

  it('starts from the last completed stop of a live route', () => {
    const plan = cheapestInsertion([stop('b', -0.15)], stop('express', -0.1), {
      ...options,
      origin: { lat: 51.5, lng: -0.05 },
    });

    expect(plan?.position).toBe(0);
    expect(plan?.addedKm).toBeLessThan(0.05);
  });

  it('returns null when every position would make a stop late', () => {
    const plan = cheapestInsertion(
      [stop('a', -0.2, minutesAfterDepart(12))],
      stop('express', -0.05, minutesAfterDepart(20)),
      options,
    );

    expect(plan).toBeNull();
  });
});
