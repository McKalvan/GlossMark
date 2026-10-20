package com.tidewater.claims.intake.api

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class MoneyDto(val amountMinor: Long, val currency: String)

@Serializable
sealed class FileClaimRequest {
    abstract val policyNumber: String
    abstract val lossDate: String
    abstract val estimatedLoss: MoneyDto
    abstract val evidenceIds: List<String>
}

@Serializable
@SerialName("motor")
data class FileMotorClaim(
    override val policyNumber: String,
    override val lossDate: String,
    override val estimatedLoss: MoneyDto,
    override val evidenceIds: List<String> = emptyList(),
    val vehicleVin: String,
    val incidentKind: String,
    val policeReportNumber: String? = null,
    val thirdPartyInvolved: Boolean,
) : FileClaimRequest()

@Serializable
@SerialName("flood")
data class FileFloodClaim(
    override val policyNumber: String,
    override val lossDate: String,
    override val estimatedLoss: MoneyDto,
    override val evidenceIds: List<String> = emptyList(),
    val propertyId: String,
    val waterDepthCm: Int? = null,
    val floodSource: String,
    val inFloodZone: Boolean,
) : FileClaimRequest()

@Serializable
@SerialName("fire")
data class FileFireClaim(
    override val policyNumber: String,
    override val lossDate: String,
    override val estimatedLoss: MoneyDto,
    override val evidenceIds: List<String> = emptyList(),
    val propertyId: String,
    val fireServiceAttended: Boolean,
    val fireReportNumber: String? = null,
) : FileClaimRequest()

@Serializable
data class WithdrawClaimRequest(val reason: String)

@Serializable
data class ClaimAccepted(val claimId: String, val status: String)
