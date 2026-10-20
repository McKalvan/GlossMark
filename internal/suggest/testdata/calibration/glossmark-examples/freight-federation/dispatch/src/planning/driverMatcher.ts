export interface DriverCandidate {
  driverId: string;
  zones: readonly string[];
  shiftStart: Date;
  shiftEnd: Date;
  capacityGrams: number;
  capacityParcels: number;
}

export interface MatchableRequest {
  id: string;
  shipmentId: string;
  zone: string;
  weightGrams: number;
  parcelCount: number;
  deadline: Date | null;
  express: boolean;
}

export type UnassignedReason = 'NO_DRIVER_FOR_ZONE' | 'OUTSIDE_SHIFT' | 'CAPACITY_EXHAUSTED';

export interface Unassigned {
  request: MatchableRequest;
  reason: UnassignedReason;
}

export interface MatchResult {
  assignments: Map<string, MatchableRequest[]>;
  unassigned: Unassigned[];
}

export interface MatchOptions {
  /** Minimum time between a driver's shift start and a request's deadline. */
  minLeadMinutes: number;
}

interface Load {
  grams: number;
  parcels: number;
  zones: Set<string>;
}

/** Express first, then earliest deadline, then heaviest (bin-packing heuristic). */
export function compareForMatching(a: MatchableRequest, b: MatchableRequest): number {
  if (a.express !== b.express) return a.express ? -1 : 1;
  const da = a.deadline?.getTime() ?? Number.POSITIVE_INFINITY;
  const db = b.deadline?.getTime() ?? Number.POSITIVE_INFINITY;
  if (da !== db) return da - db;
  return b.weightGrams - a.weightGrams;
}

function headroom(driver: DriverCandidate, load: Load): number {
  return Math.min(
    (driver.capacityGrams - load.grams) / driver.capacityGrams,
    (driver.capacityParcels - load.parcels) / driver.capacityParcels,
  );
}

function fits(driver: DriverCandidate, load: Load, request: MatchableRequest): boolean {
  return (
    load.grams + request.weightGrams <= driver.capacityGrams &&
    load.parcels + request.parcelCount <= driver.capacityParcels
  );
}

function onShift(driver: DriverCandidate, request: MatchableRequest, leadMs: number): boolean {
  const start = driver.shiftStart.getTime();
  if (driver.shiftEnd.getTime() - start <= leadMs) return false;
  return request.deadline === null || start + leadMs <= request.deadline.getTime();
}

/**
 * Assigns each pickup request to one driver. A driver qualifies when they cover
 * the request's zone, their shift starts early enough to reach the request before
 * its deadline, and their vehicle still has payload and parcel capacity.
 * Among qualifying drivers, one already working the zone is preferred (compact
 * routes), then the one with most remaining capacity.
 */
// @gloss implements .DriverMatcher conf=0.82
export function matchDrivers(
  requests: readonly MatchableRequest[],
  drivers: readonly DriverCandidate[],
  options: MatchOptions,
): MatchResult {
  const loads = new Map<string, Load>(
    drivers.map((d) => [d.driverId, { grams: 0, parcels: 0, zones: new Set<string>() }]),
  );
  const loadOf = (driver: DriverCandidate): Load => {
    const load = loads.get(driver.driverId);
    if (!load) throw new Error(`no load tracked for driver ${driver.driverId}`);
    return load;
  };
  const assignments = new Map<string, MatchableRequest[]>();
  const unassigned: Unassigned[] = [];
  const leadMs = options.minLeadMinutes * 60_000;

  for (const request of [...requests].sort(compareForMatching)) {
    const inZone = drivers.filter((d) => d.zones.includes(request.zone));
    if (inZone.length === 0) {
      unassigned.push({ request, reason: 'NO_DRIVER_FOR_ZONE' });
      continue;
    }

    const available = inZone.filter((d) => onShift(d, request, leadMs));
    if (available.length === 0) {
      unassigned.push({ request, reason: 'OUTSIDE_SHIFT' });
      continue;
    }

    const withCapacity = available.filter((d) => fits(d, loadOf(d), request));
    if (withCapacity.length === 0) {
      unassigned.push({ request, reason: 'CAPACITY_EXHAUSTED' });
      continue;
    }

    const chosen = withCapacity.reduce((best, candidate) => {
      const bestLoad = loadOf(best);
      const candidateLoad = loadOf(candidate);
      const bestInZone = bestLoad.zones.has(request.zone);
      const candidateInZone = candidateLoad.zones.has(request.zone);
      if (bestInZone !== candidateInZone) return candidateInZone ? candidate : best;
      const bestRoom = headroom(best, bestLoad);
      const candidateRoom = headroom(candidate, candidateLoad);
      if (bestRoom !== candidateRoom) return candidateRoom > bestRoom ? candidate : best;
      return candidate.driverId < best.driverId ? candidate : best;
    });

    const load = loadOf(chosen);
    load.grams += request.weightGrams;
    load.parcels += request.parcelCount;
    load.zones.add(request.zone);
    assignments.set(chosen.driverId, [...(assignments.get(chosen.driverId) ?? []), request]);
  }

  return { assignments, unassigned };
}
