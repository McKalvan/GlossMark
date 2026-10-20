package com.tidewater.claims.fraud

import java.time.Duration

/** Tunables for screening. Changing one is a model-risk decision, reviewed by the SIU lead. */
// @gloss implements .FraudReferralPolicy conf=0.9
object Thresholds {
    /** A claim scoring at or above this is held for the special investigations unit. */
    const val REFERRAL_SCORE_THRESHOLD = 0.75

    /** Repeat claims by one policyholder inside this window add to the score. */
    val REPEAT_CLAIM_WINDOW: Duration = Duration.ofDays(365)

    /** A loss reported this soon after the policy started is a classic early-claim signal. */
    val EARLY_CLAIM_WINDOW: Duration = Duration.ofDays(30)
}
