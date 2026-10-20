package com.tidewater.claims.intake.withdrawal

import com.tidewater.claims.intake.domain.ClaimRepository
import com.tidewater.claims.intake.domain.ClaimStatus
import com.tidewater.claims.intake.events.ClaimEventPublisher
import java.time.Clock
import java.util.UUID

/** A policyholder withdraws a claim that has not been decided yet; the claim is closed and consumers told. */
// @gloss implements .WithdrawalHandler conf=0.9
class WithdrawalService(
    private val claims: ClaimRepository,
    private val publisher: ClaimEventPublisher,
    private val clock: Clock,
) {
    enum class Outcome { WITHDRAWN, NOT_FOUND, ALREADY_DECIDED }

    fun withdraw(id: UUID, reason: String): Outcome {
        val status = claims.status(id) ?: return Outcome.NOT_FOUND
        if (status != ClaimStatus.FILED || !claims.markWithdrawn(id)) return Outcome.ALREADY_DECIDED
        publisher.publishWithdrawn(claims.header(id), reason, clock.instant())
        return Outcome.WITHDRAWN
    }
}
