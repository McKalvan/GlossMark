package com.tidewater.claims.fraud

import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

// @gloss verifies .FraudScorer conf=0.55
class FraudScorerTest {
    private val scorer = FraudScorer()
    private val filed = Instant.parse("2026-10-01T09:00:00Z")

    private fun input(
        eventType: String = "MotorClaimFiled",
        lossMinor: Long = 100_000,
        policyStart: Instant? = Instant.parse("2024-01-01T00:00:00Z"),
        prior: Int = 0,
        report: Boolean = true,
        evidence: Int = 2,
    ) = ScreeningInput("c-1", "h-1", eventType, lossMinor, filed, policyStart, prior, report, evidence)

    @Test
    fun `an ordinary claim is not referred`() {
        val score = scorer.score(input())
        assertEquals(0.0, score.value)
        assertFalse(score.refer)
    }

    // @gloss verifies claim:FraudReferralThreshold conf=0.85
    @Test
    fun `early claim plus high value without a police report crosses the threshold`() {
        val score = scorer.score(input(lossMinor = 900_000, policyStart = filed.minusSeconds(86_400 * 10), report = false))
        assertTrue(score.refer, "score ${score.value} should be at or above ${Thresholds.REFERRAL_SCORE_THRESHOLD}")
        assertEquals(listOf("EARLY_CLAIM", "HIGH_VALUE_NO_REPORT"), score.signals)
    }

    @Test
    fun `a fire with no fire report and no evidence is suspicious but below the threshold alone`() {
        val score = scorer.score(input(eventType = "FireClaimFiled", report = false, evidence = 0))
        assertEquals(0.50, score.value, 1e-9)
        assertFalse(score.refer)
    }
}
