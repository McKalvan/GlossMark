import { haversineKm, travelMinutes, type LatLng } from './geo.js';

export interface OptimizerStop extends LatLng {
  id: string;
  /** Latest acceptable arrival, derived from the shipment's delivery window. Null = unconstrained. */
  deadline: Date | null;
}

export interface OptimizerOptions {
  /** Where the route ends (the driver returns parcels to the depot for linehaul). */
  depot: LatLng;
  /** Where the route starts; defaults to the depot. Live routes start at the last completed stop. */
  origin?: LatLng;
  departAt: Date;
  avgSpeedKmh: number;
  serviceMinutes: number;
  maxPasses?: number;
}

export interface ScheduledStop {
  id: string;
  seq: number;
  arrivalAt: Date;
  late: boolean;
}

export interface RouteSchedule {
  stops: ScheduledStop[];
  distanceKm: number;
  lateStopIds: string[];
}

const MS_PER_MINUTE = 60_000;
const URGENT_SLACK_MINUTES = 45;
const DEFAULT_MAX_PASSES = 50;
const IMPROVEMENT_EPSILON_KM = 1e-6;

/** Index 0 is the origin, 1..n are the stops, n+1 is the depot. */
class DistanceMatrix {
  private readonly km: number[][];
  readonly end: number;

  constructor(origin: LatLng, stops: readonly LatLng[], depot: LatLng) {
    const points = [origin, ...stops, depot];
    this.km = points.map((a) => points.map((b) => haversineKm(a, b)));
    this.end = points.length - 1;
  }

  between(from: number, to: number): number {
    return this.km[from][to];
  }
}

interface Evaluation {
  distanceKm: number;
  arrivals: number[];
  lateCount: number;
}

function evaluate(
  order: readonly number[],
  stops: readonly OptimizerStop[],
  matrix: DistanceMatrix,
  opts: OptimizerOptions,
): Evaluation {
  let position = 0;
  let clock = opts.departAt.getTime();
  let distanceKm = 0;
  let lateCount = 0;
  const arrivals: number[] = [];

  for (const k of order) {
    const node = k + 1;
    const leg = matrix.between(position, node);
    distanceKm += leg;
    clock += travelMinutes(leg, opts.avgSpeedKmh) * MS_PER_MINUTE;
    arrivals.push(clock);
    const deadline = stops[k].deadline;
    if (deadline !== null && clock > deadline.getTime()) lateCount += 1;
    clock += opts.serviceMinutes * MS_PER_MINUTE;
    position = node;
  }

  distanceKm += matrix.between(position, matrix.end);
  return { distanceKm, arrivals, lateCount };
}

/**
 * Greedy construction: always drive to the nearest remaining stop, unless a stop's
 * slack against its deadline has dropped below URGENT_SLACK_MINUTES, in which case
 * the most urgent stop goes next.
 */
function nearestNeighbour(
  stops: readonly OptimizerStop[],
  matrix: DistanceMatrix,
  opts: OptimizerOptions,
): number[] {
  const remaining = new Set(stops.map((_, k) => k));
  const order: number[] = [];
  let position = 0;
  let clock = opts.departAt.getTime();

  while (remaining.size > 0) {
    let nearest = -1;
    let nearestKm = Number.POSITIVE_INFINITY;
    let urgent = -1;
    let urgentSlack = Number.POSITIVE_INFINITY;

    for (const k of remaining) {
      const leg = matrix.between(position, k + 1);
      if (leg < nearestKm) {
        nearest = k;
        nearestKm = leg;
      }
      const deadline = stops[k].deadline;
      if (deadline !== null) {
        const arrival = clock + travelMinutes(leg, opts.avgSpeedKmh) * MS_PER_MINUTE;
        const slack = deadline.getTime() - arrival;
        if (slack < URGENT_SLACK_MINUTES * MS_PER_MINUTE && slack < urgentSlack) {
          urgent = k;
          urgentSlack = slack;
        }
      }
    }

    const next = urgent >= 0 ? urgent : nearest;
    const leg = matrix.between(position, next + 1);
    clock += (travelMinutes(leg, opts.avgSpeedKmh) + opts.serviceMinutes) * MS_PER_MINUTE;
    position = next + 1;
    remaining.delete(next);
    order.push(next);
  }

  return order;
}

/**
 * 2-opt local search: reverse every segment [i..k] and keep the result when it
 * makes fewer stops late, or the same number of stops late over a shorter distance.
 * A move that makes more stops late is never accepted.
 */
function twoOpt(
  initial: number[],
  stops: readonly OptimizerStop[],
  matrix: DistanceMatrix,
  opts: OptimizerOptions,
): number[] {
  const maxPasses = opts.maxPasses ?? DEFAULT_MAX_PASSES;
  let best = initial;
  let bestEval = evaluate(best, stops, matrix, opts);
  let improved = true;
  let passes = 0;

  while (improved && passes < maxPasses) {
    improved = false;
    passes += 1;
    for (let i = 0; i < best.length - 1; i += 1) {
      for (let k = i + 1; k < best.length; k += 1) {
        const candidate = [
          ...best.slice(0, i),
          ...best.slice(i, k + 1).reverse(),
          ...best.slice(k + 1),
        ];
        const candidateEval = evaluate(candidate, stops, matrix, opts);
        const fewerLate = candidateEval.lateCount < bestEval.lateCount;
        const shorter =
          candidateEval.lateCount === bestEval.lateCount &&
          candidateEval.distanceKm < bestEval.distanceKm - IMPROVEMENT_EPSILON_KM;
        if (fewerLate || shorter) {
          best = candidate;
          bestEval = candidateEval;
          improved = true;
        }
      }
    }
  }

  return best;
}

function toSchedule(
  order: readonly number[],
  stops: readonly OptimizerStop[],
  matrix: DistanceMatrix,
  opts: OptimizerOptions,
): RouteSchedule {
  const { distanceKm, arrivals } = evaluate(order, stops, matrix, opts);
  const scheduled = order.map((k, i): ScheduledStop => {
    const arrivalAt = new Date(arrivals[i]);
    const deadline = stops[k].deadline;
    return {
      id: stops[k].id,
      seq: i + 1,
      arrivalAt,
      late: deadline !== null && arrivalAt.getTime() > deadline.getTime(),
    };
  });
  return {
    stops: scheduled,
    distanceKm,
    lateStopIds: scheduled.filter((s) => s.late).map((s) => s.id),
  };
}

/** Orders the stops (nearest-neighbour, then 2-opt) and schedules arrivals. */
// @gloss implements .RouteOptimizer conf=0.82
export function optimizeRoute(stops: readonly OptimizerStop[], opts: OptimizerOptions): RouteSchedule {
  if (stops.length === 0) return { stops: [], distanceKm: 0, lateStopIds: [] };
  const matrix = new DistanceMatrix(opts.origin ?? opts.depot, stops, opts.depot);
  const order = twoOpt(nearestNeighbour(stops, matrix, opts), stops, matrix, opts);
  return toSchedule(order, stops, matrix, opts);
}

/** Schedules arrivals for stops in the order given, without reordering them. */
export function scheduleRoute(stops: readonly OptimizerStop[], opts: OptimizerOptions): RouteSchedule {
  const matrix = new DistanceMatrix(opts.origin ?? opts.depot, stops, opts.depot);
  return toSchedule(
    stops.map((_, k) => k),
    stops,
    matrix,
    opts,
  );
}

/** The ETA window shown to the shipper, centred on the predicted arrival. */
export function etaWindow(arrivalAt: Date, windowMinutes: number): { etaFrom: Date; etaTo: Date } {
  const half = (windowMinutes / 2) * MS_PER_MINUTE;
  return {
    etaFrom: new Date(arrivalAt.getTime() - half),
    etaTo: new Date(arrivalAt.getTime() + half),
  };
}
