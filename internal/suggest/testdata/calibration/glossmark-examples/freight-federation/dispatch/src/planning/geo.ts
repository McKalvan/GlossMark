export interface LatLng {
  lat: number;
  lng: number;
}

const EARTH_RADIUS_KM = 6371.0088;

/**
 * Road distance in London is on average ~1.3x the great-circle distance. Applied to
 * travel time only; reported distances stay great-circle so they are comparable.
 */
const CIRCUITY_FACTOR = 1.3;

const toRadians = (degrees: number): number => (degrees * Math.PI) / 180;

export function haversineKm(a: LatLng, b: LatLng): number {
  const dLat = toRadians(b.lat - a.lat);
  const dLng = toRadians(b.lng - a.lng);
  const h =
    Math.sin(dLat / 2) ** 2 +
    Math.cos(toRadians(a.lat)) * Math.cos(toRadians(b.lat)) * Math.sin(dLng / 2) ** 2;
  return 2 * EARTH_RADIUS_KM * Math.asin(Math.min(1, Math.sqrt(h)));
}

export function travelMinutes(distanceKm: number, avgSpeedKmh: number): number {
  return ((distanceKm * CIRCUITY_FACTOR) / avgSpeedKmh) * 60;
}

/** Zones are UK postcode areas: the leading letters of the outward code ("EC1A 1BB" -> "EC"). */
export function zoneForPostcode(postcode: string): string {
  const outward = postcode.trim().toUpperCase().split(/\s+/)[0] ?? '';
  const area = /^[A-Z]{1,2}/.exec(outward);
  return area ? area[0] : 'UNZONED';
}
