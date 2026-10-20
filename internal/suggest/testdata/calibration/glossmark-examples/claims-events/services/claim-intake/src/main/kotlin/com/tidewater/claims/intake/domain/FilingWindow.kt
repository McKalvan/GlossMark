package com.tidewater.claims.intake.domain

import java.time.Clock
import java.time.LocalDate
import java.time.temporal.ChronoUnit

/** Days after the loss within which a claim must be filed. Later filings are refused at intake. */
// @gloss publishes-value .FilingWindowRule scope=line conf=0.9
const val FILING_WINDOW_DAYS = 60L

class LateFilingException(val daysLate: Long) :
    IllegalArgumentException("claim filed $daysLate days after the filing window closed")

// @gloss implements .FilingWindowRule conf=0.9
fun requireWithinFilingWindow(lossDate: LocalDate, clock: Clock) {
    val today = LocalDate.now(clock)
    require(!lossDate.isAfter(today)) { "loss date is in the future" }
    val elapsed = ChronoUnit.DAYS.between(lossDate, today)
    if (elapsed > FILING_WINDOW_DAYS) throw LateFilingException(elapsed - FILING_WINDOW_DAYS)
}
