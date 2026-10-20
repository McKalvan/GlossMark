// @gloss verifies .DeliveryConfirmation scope=file conf=0.9
use chrono::{DateTime, Duration, TimeZone, Utc};
use kestrel_tracking::delivery::confirmation::{
    decide_delivery, ConfirmationError, DeliveryContext, ProofOfDelivery,
};
use kestrel_tracking::delivery::geofence::Coordinate;
use kestrel_tracking::events::types::{Proof, ProofKind};
use uuid::Uuid;

const DESTINATION: Coordinate = Coordinate::new(53.4808, -2.2426);
const DEG_PER_M: f64 = 1.0 / 111_195.0;

fn promised_start() -> DateTime<Utc> {
    Utc.with_ymd_and_hms(2026, 10, 19, 13, 0, 0).unwrap()
}

fn context() -> DeliveryContext {
    DeliveryContext {
        shipment_id: Uuid::parse_str("6f1c2b1e-8a54-4d2f-9a0e-3c2b7f1d9e10").unwrap(),
        tracking_number: "KF12345678903".to_owned(),
        promised_start: promised_start(),
        destination: DESTINATION,
    }
}

fn pod(kind: ProofKind, reference: &str, metres_away: f64, after_start: Duration) -> ProofOfDelivery {
    ProofOfDelivery {
        delivered_at: promised_start() + after_start,
        position: Coordinate::new(DESTINATION.lat + metres_away * DEG_PER_M, DESTINATION.lng),
        proof: Proof {
            kind,
            reference: reference.to_owned(),
        },
    }
}

#[test]
fn signature_inside_window_is_on_time() {
    let event = decide_delivery(
        &context(),
        &pod(ProofKind::Signature, "sig/2026/10/19/a81f.png", 5.0, Duration::minutes(45)),
    )
    .unwrap();

    assert!(event.within_window);
    assert_eq!(event.tracking_number, "KF12345678903");
    assert_eq!(event.proof.kind, ProofKind::Signature);
}

#[test]
fn photo_after_window_is_late() {
    let event = decide_delivery(
        &context(),
        &pod(ProofKind::Photo, "pod/2026/10/19/c4d2.jpg", 10.0, Duration::minutes(121)),
    )
    .unwrap();

    assert!(!event.within_window);
}

#[test]
fn signature_does_not_require_geofence() {
    let event = decide_delivery(
        &context(),
        &pod(ProofKind::Signature, "sig/2026/10/19/ff01.png", 400.0, Duration::minutes(30)),
    );
    assert!(event.is_ok());
}

#[test]
fn geofence_proof_inside_radius_is_accepted() {
    let event = decide_delivery(
        &context(),
        &pod(ProofKind::Geofence, "geofence:hh-0042:2026-10-19T13:30:00Z", 60.0, Duration::minutes(30)),
    )
    .unwrap();
    assert_eq!(event.proof.kind, ProofKind::Geofence);
}

#[test]
fn geofence_proof_outside_radius_is_rejected() {
    let err = decide_delivery(
        &context(),
        &pod(ProofKind::Geofence, "geofence:hh-0042:2026-10-19T13:30:00Z", 200.0, Duration::minutes(30)),
    )
    .unwrap_err();

    match err {
        ConfirmationError::OutsideGeofence { distance_m, radius_m } => {
            assert!((distance_m - 200.0).abs() < 1.0);
            assert_eq!(radius_m, 75.0);
        }
        other => panic!("unexpected error {other:?}"),
    }
}

#[test]
fn empty_proof_reference_is_rejected() {
    let err = decide_delivery(&context(), &pod(ProofKind::Photo, "  ", 0.0, Duration::minutes(10)))
        .unwrap_err();
    assert_eq!(err, ConfirmationError::EmptyProofReference);
}

#[test]
fn shipment_delivered_serializes_in_contract_shape() {
    let event = decide_delivery(
        &context(),
        &pod(ProofKind::Geofence, "geofence:hh-0042:2026-10-19T13:30:00Z", 0.0, Duration::minutes(30)),
    )
    .unwrap();

    let json = serde_json::to_value(&event).unwrap();
    assert_eq!(json["trackingNumber"], "KF12345678903");
    assert_eq!(json["withinWindow"], true);
    assert_eq!(json["proof"]["kind"], "GEOFENCE");
    assert_eq!(json["proof"]["ref"], "geofence:hh-0042:2026-10-19T13:30:00Z");
    assert!(json.get("deliveredAt").is_some());
}
