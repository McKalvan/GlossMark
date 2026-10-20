package com.tidewater.claims.intake

import com.tidewater.claims.intake.domain.FILING_WINDOW_DAYS
import com.tidewater.claims.intake.domain.LateFilingException
import com.tidewater.claims.intake.domain.requireWithinFilingWindow
import java.time.Clock
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneOffset
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

// @gloss verifies claim:FilingWindow conf=0.55
class FilingWindowTest {
    private val clock = Clock.fixed(Instant.parse("2026-10-01T09:00:00Z"), ZoneOffset.UTC)
    private val today = LocalDate.of(2026, 10, 1)

    @Test
    fun `a claim filed on the last day of the window is accepted`() {
        requireWithinFilingWindow(today.minusDays(FILING_WINDOW_DAYS), clock)
    }

    @Test
    fun `a claim filed one day late is refused with the days late`() {
        val e = assertFailsWith<LateFilingException> {
            requireWithinFilingWindow(today.minusDays(FILING_WINDOW_DAYS + 1), clock)
        }
        assertEquals(1, e.daysLate)
    }

    @Test
    fun `a loss in the future is refused`() {
        assertFailsWith<IllegalArgumentException> { requireWithinFilingWindow(today.plusDays(1), clock) }
    }
}
