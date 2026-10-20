//! The delivery window promised to the customer: two hours from the promised
//! start. Tracking takes the promised start from the planned stop's `etaFrom`.

use std::time::Duration;

use chrono::{DateTime, Utc};

pub const DELIVERY_WINDOW: Duration = Duration::from_secs(2 * 60 * 60);

/// True when `delivered_at` falls no later than [`DELIVERY_WINDOW`] after
/// `promised_start`. The end of the window is inclusive. A delivery made
/// before the window opens counts as on time.
pub fn within_window(promised_start: DateTime<Utc>, delivered_at: DateTime<Utc>) -> bool {
    match delivered_at.signed_duration_since(promised_start).to_std() {
        Ok(elapsed) => elapsed <= DELIVERY_WINDOW,
        Err(_) => true,
    }
}

pub fn window_end(promised_start: DateTime<Utc>) -> DateTime<Utc> {
    promised_start + chrono::Duration::from_std(DELIVERY_WINDOW).expect("delivery window fits in chrono::Duration")
}
