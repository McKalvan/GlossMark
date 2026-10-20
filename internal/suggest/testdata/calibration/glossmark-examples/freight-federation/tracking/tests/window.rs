// @gloss verifies shc:DeliveryWindowPolicy scope=file conf=0.9 note="the two-hour promise as tracking applies it; the window is owned by shipment-core"
use chrono::{DateTime, Duration, TimeZone, Utc};
use kestrel_tracking::delivery::window::{window_end, within_window, DELIVERY_WINDOW};

fn promised_start() -> DateTime<Utc> {
    Utc.with_ymd_and_hms(2026, 10, 19, 9, 0, 0).unwrap()
}

#[test]
fn window_is_two_hours() {
    assert_eq!(DELIVERY_WINDOW.as_secs(), 7_200);
}

#[test]
fn delivered_at_1h59_is_on_time() {
    let delivered_at = promised_start() + Duration::minutes(119);
    assert!(within_window(promised_start(), delivered_at));
}

#[test]
fn delivered_at_2h01_is_late() {
    let delivered_at = promised_start() + Duration::minutes(121);
    assert!(!within_window(promised_start(), delivered_at));
}

#[test]
fn window_end_is_inclusive() {
    let delivered_at = promised_start() + Duration::hours(2);
    assert!(within_window(promised_start(), delivered_at));
    assert!(!within_window(promised_start(), delivered_at + Duration::seconds(1)));
}

#[test]
fn early_delivery_counts_as_on_time() {
    let delivered_at = promised_start() - Duration::minutes(20);
    assert!(within_window(promised_start(), delivered_at));
}

#[test]
fn window_end_is_start_plus_two_hours() {
    assert_eq!(
        window_end(promised_start()),
        Utc.with_ymd_and_hms(2026, 10, 19, 11, 0, 0).unwrap()
    );
}
