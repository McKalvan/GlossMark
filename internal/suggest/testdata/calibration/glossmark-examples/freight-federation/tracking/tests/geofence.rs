// @gloss verifies .GeofenceArrival, .ArrivalRadius scope=file conf=0.9
use kestrel_tracking::delivery::geofence::{
    detect_arrival, haversine_m, is_within_arrival_radius, Coordinate, ARRIVAL_RADIUS_M,
};

const DELIVERY_ADDRESS: Coordinate = Coordinate::new(51.5074, -0.1278);

/// Roughly one metre of latitude in degrees.
const DEG_PER_M: f64 = 1.0 / 111_195.0;

fn north_of(origin: Coordinate, metres: f64) -> Coordinate {
    Coordinate::new(origin.lat + metres * DEG_PER_M, origin.lng)
}

#[test]
fn arrival_radius_is_75_metres() {
    assert_eq!(ARRIVAL_RADIUS_M, 75.0);
}

#[test]
fn haversine_is_zero_for_same_point() {
    assert_eq!(haversine_m(DELIVERY_ADDRESS, DELIVERY_ADDRESS), 0.0);
}

#[test]
fn haversine_london_to_paris() {
    let paris = Coordinate::new(48.8566, 2.3522);
    let d = haversine_m(DELIVERY_ADDRESS, paris);
    assert!((d - 343_500.0).abs() < 2_000.0, "got {d}");
}

#[test]
fn haversine_is_symmetric() {
    let other = north_of(DELIVERY_ADDRESS, 500.0);
    assert_eq!(haversine_m(DELIVERY_ADDRESS, other), haversine_m(other, DELIVERY_ADDRESS));
}

#[test]
fn driver_70m_away_has_arrived() {
    let ping = north_of(DELIVERY_ADDRESS, 70.0);
    let arrival = detect_arrival(ping, Some(8.0), DELIVERY_ADDRESS).expect("should arrive");
    assert!((arrival.distance_m - 70.0).abs() < 0.5);
    assert!(is_within_arrival_radius(ping, DELIVERY_ADDRESS));
}

#[test]
fn driver_80m_away_has_not_arrived() {
    let ping = north_of(DELIVERY_ADDRESS, 80.0);
    assert_eq!(detect_arrival(ping, Some(8.0), DELIVERY_ADDRESS), None);
    assert!(!is_within_arrival_radius(ping, DELIVERY_ADDRESS));
}

#[test]
fn inaccurate_ping_cannot_prove_arrival() {
    let ping = north_of(DELIVERY_ADDRESS, 10.0);
    assert_eq!(detect_arrival(ping, Some(120.0), DELIVERY_ADDRESS), None);
}

#[test]
fn ping_without_reported_accuracy_is_trusted() {
    let ping = north_of(DELIVERY_ADDRESS, 30.0);
    assert!(detect_arrival(ping, None, DELIVERY_ADDRESS).is_some());
}

#[test]
fn null_island_is_not_a_valid_position() {
    assert!(!Coordinate::new(0.0, 0.0).is_valid());
    assert_eq!(detect_arrival(Coordinate::new(0.0, 0.0), None, DELIVERY_ADDRESS), None);
}
