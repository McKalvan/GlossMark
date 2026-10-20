//! Geofence arrival: a driver has arrived at a stop once a GPS ping places
//! them within the arrival radius of the delivery address.

/// Mean Earth radius (IUGG), metres.
pub const EARTH_RADIUS_M: f64 = 6_371_008.8;

pub const ARRIVAL_RADIUS_M: f64 = 75.0;

/// Pings reporting a worse horizontal accuracy than this cannot prove
/// arrival: the true position could be anywhere inside the error circle.
pub const MAX_ARRIVAL_ACCURACY_M: f32 = 50.0;

#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Coordinate {
    pub lat: f64,
    pub lng: f64,
}

impl Coordinate {
    pub const fn new(lat: f64, lng: f64) -> Self {
        Self { lat, lng }
    }

    pub fn is_valid(&self) -> bool {
        self.lat.is_finite()
            && self.lng.is_finite()
            && (-90.0..=90.0).contains(&self.lat)
            && (-180.0..=180.0).contains(&self.lng)
            && !(self.lat == 0.0 && self.lng == 0.0)
    }
}

/// Great-circle distance between two coordinates in metres (haversine).
pub fn haversine_m(a: Coordinate, b: Coordinate) -> f64 {
    let (lat1, lat2) = (a.lat.to_radians(), b.lat.to_radians());
    let d_lat = (b.lat - a.lat).to_radians();
    let d_lng = (b.lng - a.lng).to_radians();

    let h = (d_lat / 2.0).sin().powi(2) + lat1.cos() * lat2.cos() * (d_lng / 2.0).sin().powi(2);
    2.0 * EARTH_RADIUS_M * h.sqrt().min(1.0).asin()
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Arrival {
    pub distance_m: f64,
}

/// Returns `Some(Arrival)` when `position` lies within [`ARRIVAL_RADIUS_M`]
/// of `destination` (boundary inclusive) and the ping is accurate enough to
/// be trusted. `accuracy_m` of `None` means the device did not report one.
// @gloss implements .GeofenceArrival conf=0.9
pub fn detect_arrival(
    position: Coordinate,
    accuracy_m: Option<f32>,
    destination: Coordinate,
) -> Option<Arrival> {
    if !position.is_valid() || !destination.is_valid() {
        return None;
    }
    if accuracy_m.is_some_and(|acc| acc > MAX_ARRIVAL_ACCURACY_M) {
        return None;
    }
    let distance_m = haversine_m(position, destination);
    (distance_m <= ARRIVAL_RADIUS_M).then_some(Arrival { distance_m })
}

pub fn is_within_arrival_radius(position: Coordinate, destination: Coordinate) -> bool {
    haversine_m(position, destination) <= ARRIVAL_RADIUS_M
}
