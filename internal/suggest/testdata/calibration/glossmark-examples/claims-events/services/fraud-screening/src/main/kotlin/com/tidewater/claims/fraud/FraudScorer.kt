package com.tidewater.claims.fraud

import java.time.Duration
import java.time.Instant

/** What the scorer knows about a filed claim, whatever its line of business. */
data class ScreeningInput(
    val claimId: String,
    val policyholderId: String,
    val eventType: String,
    val estimatedLossMinor: Long,
    val filedAt: Instant,
    val policyStartedAt: Instant?,
    val priorClaimsInWindow: Int,
    val hasPoliceOrFireReport: Boolean,
    val evidenceCount: Int,
)

data class Score(val value: Double, val signals: List<String>) {
    val refer: Boolean get() = value >= Thresholds.REFERRAL_SCORE_THRESHOLD
}

/** Rule-based screening: each signal adds weight; the sum is capped at 1.0. */
// @gloss implements .FraudScorer conf=0.95
class FraudScorer {
    fun score(input: ScreeningInput): Score {
        val signals = buildList {
            if (input.policyStartedAt != null &&
                Duration.between(input.policyStartedAt, input.filedAt) < Thresholds.EARLY_CLAIM_WINDOW
            ) add("EARLY_CLAIM" to 0.45)
            if (input.priorClaimsInWindow >= 2) add("REPEAT_CLAIMANT" to 0.25 + 0.05 * (input.priorClaimsInWindow - 2))
            if (input.eventType == "MotorClaimFiled" && !input.hasPoliceOrFireReport && input.estimatedLossMinor > 500_000) {
                add("HIGH_VALUE_NO_REPORT" to 0.30)
            }
            if (input.eventType == "FireClaimFiled" && !input.hasPoliceOrFireReport) add("FIRE_NO_REPORT" to 0.35)
            if (input.evidenceCount == 0) add("NO_EVIDENCE" to 0.15)
        }
        return Score(signals.sumOf { it.second }.coerceAtMost(1.0), signals.map { it.first })
    }
}
