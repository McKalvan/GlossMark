package com.tidewater.claims.intake.events

import com.tidewater.claims.common.ClaimRef
import com.tidewater.claims.common.LineOfBusiness
import com.tidewater.claims.events.FireClaimFiled
import com.tidewater.claims.events.FloodClaimFiled
import com.tidewater.claims.events.MotorClaimFiled
import com.tidewater.claims.events.MotorIncidentKind
import com.tidewater.claims.events.FloodSource
import com.tidewater.claims.intake.domain.Claim
import com.tidewater.claims.intake.domain.FireClaim
import com.tidewater.claims.intake.domain.FloodClaim
import com.tidewater.claims.intake.domain.Money
import com.tidewater.claims.intake.domain.MotorClaim
import org.apache.avro.specific.SpecificRecord
import java.util.UUID
import com.tidewater.claims.common.Money as AvroMoney

internal fun Money.toAvro(): AvroMoney = AvroMoney(amountMinor, currency)

internal fun Claim.ref(): ClaimRef = ClaimRef(
    id.toString(),
    policyNumber,
    policyholderId.toString(),
    if (this is MotorClaim) LineOfBusiness.MOTOR else LineOfBusiness.HOME,
)

/** Maps a domain claim to the Avro record for its line of business. */
internal fun Claim.toFiledEvent(): SpecificRecord = when (this) {
    is MotorClaim -> MotorClaimFiled.newBuilder()
        .setEventId(UUID.randomUUID().toString())
        .setOccurredAt(filedAt)
        .setClaim(ref())
        .setLossDate(lossDate)
        .setEstimatedLoss(estimatedLoss.toAvro())
        .setVehicleVin(vehicleVin)
        .setIncidentKind(MotorIncidentKind.valueOf(incidentKind))
        .setPoliceReportNumber(policeReportNumber)
        .setThirdPartyInvolved(thirdPartyInvolved)
        .setEvidenceIds(evidenceIds)
        .build()
    is FloodClaim -> FloodClaimFiled.newBuilder()
        .setEventId(UUID.randomUUID().toString())
        .setOccurredAt(filedAt)
        .setClaim(ref())
        .setLossDate(lossDate)
        .setEstimatedLoss(estimatedLoss.toAvro())
        .setPropertyId(propertyId)
        .setWaterDepthCm(waterDepthCm)
        .setFloodSource(FloodSource.valueOf(floodSource))
        .setInFloodZone(inFloodZone)
        .setEvidenceIds(evidenceIds)
        .build()
    is FireClaim -> FireClaimFiled.newBuilder()
        .setEventId(UUID.randomUUID().toString())
        .setOccurredAt(filedAt)
        .setClaim(ref())
        .setLossDate(lossDate)
        .setEstimatedLoss(estimatedLoss.toAvro())
        .setPropertyId(propertyId)
        .setFireServiceAttended(fireServiceAttended)
        .setFireReportNumber(fireReportNumber)
        .setEvidenceIds(evidenceIds)
        .build()
}
