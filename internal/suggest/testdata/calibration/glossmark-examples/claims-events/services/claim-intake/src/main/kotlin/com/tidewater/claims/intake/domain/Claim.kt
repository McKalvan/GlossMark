package com.tidewater.claims.intake.domain

import java.time.Instant
import java.time.LocalDate
import java.util.UUID

data class Money(val amountMinor: Long, val currency: String) {
    init {
        require(amountMinor >= 0) { "amount must not be negative" }
        require(currency.length == 3) { "currency must be ISO 4217" }
    }
}

enum class ClaimStatus { FILED, WITHDRAWN, APPROVED, DENIED, REFERRED }

/** A request for indemnity under a policy. Each line of business adds what it needs to assess the loss. */
// @gloss represents claim:Claim conf=0.95
sealed class Claim {
    abstract val id: UUID
    abstract val policyNumber: String
    abstract val policyholderId: UUID
    abstract val lossDate: LocalDate
    abstract val estimatedLoss: Money
    abstract val filedAt: Instant
    abstract val status: ClaimStatus
    abstract val evidenceIds: List<String>
}

// @gloss represents claim:MotorClaim conf=0.9
data class MotorClaim(
    override val id: UUID,
    override val policyNumber: String,
    override val policyholderId: UUID,
    override val lossDate: LocalDate,
    override val estimatedLoss: Money,
    override val filedAt: Instant,
    override val status: ClaimStatus,
    override val evidenceIds: List<String>,
    val vehicleVin: String,
    val incidentKind: String,
    val policeReportNumber: String?,
    val thirdPartyInvolved: Boolean,
) : Claim()

/** Damage to insured property. */
// @gloss represents claim:PropertyClaim conf=0.9
sealed class PropertyClaim : Claim() {
    abstract val propertyId: String
}

/** A property claim on a household policy. */
// @gloss represents claim:HomeClaim conf=0.9
sealed class HomeClaim : PropertyClaim()

// @gloss represents claim:FloodClaim conf=0.9
data class FloodClaim(
    override val id: UUID,
    override val policyNumber: String,
    override val policyholderId: UUID,
    override val lossDate: LocalDate,
    override val estimatedLoss: Money,
    override val filedAt: Instant,
    override val status: ClaimStatus,
    override val evidenceIds: List<String>,
    override val propertyId: String,
    val waterDepthCm: Int?,
    val floodSource: String,
    val inFloodZone: Boolean,
) : HomeClaim()

// @gloss represents claim:FireClaim conf=0.9
data class FireClaim(
    override val id: UUID,
    override val policyNumber: String,
    override val policyholderId: UUID,
    override val lossDate: LocalDate,
    override val estimatedLoss: Money,
    override val filedAt: Instant,
    override val status: ClaimStatus,
    override val evidenceIds: List<String>,
    override val propertyId: String,
    val fireServiceAttended: Boolean,
    val fireReportNumber: String?,
) : HomeClaim()
