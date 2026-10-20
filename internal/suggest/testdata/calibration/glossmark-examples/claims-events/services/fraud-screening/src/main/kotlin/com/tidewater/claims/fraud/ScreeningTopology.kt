package com.tidewater.claims.fraud

import com.tidewater.claims.events.FraudSuspected
import org.apache.avro.generic.GenericRecord
import org.apache.avro.specific.SpecificRecord
import org.apache.kafka.common.serialization.Serde
import org.apache.kafka.common.serialization.Serdes
import org.apache.kafka.streams.KeyValue
import org.apache.kafka.streams.StreamsBuilder
import org.apache.kafka.streams.Topology
import org.apache.kafka.streams.kstream.Consumed
import org.apache.kafka.streams.kstream.Grouped
import org.apache.kafka.streams.kstream.Materialized
import org.apache.kafka.streams.kstream.Produced
import java.time.Instant
import java.util.UUID

const val FILED_TOPIC = "claims.filed.v1"
const val ALERTS_TOPIC = "fraud.alerts.v1"

/**
 * Reads every filed claim, keeps a per-policyholder claim count, scores each claim and
 * publishes FraudSuspected for the ones at or above the referral threshold.
 */
// @gloss implements .ScreeningTopology conf=0.95
// @gloss subscribes claim:ClaimFiled conf=0.9
fun screeningTopology(
    filedSerde: Serde<GenericRecord>,
    alertSerde: Serde<SpecificRecord>,
    scorer: FraudScorer = FraudScorer(),
): Topology {
    val builder = StreamsBuilder()
    val filed = builder.stream(FILED_TOPIC, Consumed.with(Serdes.String(), filedSerde))

    val priorClaims = filed
        .groupBy({ _, v -> v.ref("policyholderId") }, Grouped.with(Serdes.String(), filedSerde))
        .count(Materialized.`as`("claims-per-policyholder"))

    filed
        .selectKey { _, v -> v.ref("policyholderId") }
        .join(priorClaims) { claim, count -> claim to count.toInt() - 1 }
        .map { _, (claim, prior) -> KeyValue(claim.ref("claimId"), claim to scorer.score(claim.toInput(prior))) }
        .filter { _, (_, score) -> score.refer }
        .mapValues { (claim, score) -> alert(claim, score) as SpecificRecord }
        .to(ALERTS_TOPIC, Produced.with(Serdes.String(), alertSerde))

    return builder.build()
}

private fun GenericRecord.ref(field: String): String = (get("claim") as GenericRecord).get(field).toString()

private fun GenericRecord.toInput(prior: Int) = ScreeningInput(
    claimId = ref("claimId"),
    policyholderId = ref("policyholderId"),
    eventType = schema.name,
    estimatedLossMinor = (get("estimatedLoss") as GenericRecord).get("amountMinor") as Long,
    filedAt = Instant.ofEpochMilli(get("occurredAt") as Long),
    policyStartedAt = null,
    priorClaimsInWindow = prior,
    hasPoliceOrFireReport = listOf("policeReportNumber", "fireReportNumber").any { schema.getField(it) != null && get(it) != null },
    evidenceCount = (get("evidenceIds") as List<*>).size,
)

// @gloss publishes claim:FraudSuspected conf=0.9
private fun alert(claim: GenericRecord, score: Score): FraudSuspected = FraudSuspected.newBuilder()
    .setEventId(UUID.randomUUID().toString())
    .setOccurredAt(Instant.now())
    .setClaim(com.tidewater.claims.common.ClaimRef.newBuilder()
        .setClaimId(claim.ref("claimId"))
        .setPolicyNumber(claim.ref("policyNumber"))
        .setPolicyholderId(claim.ref("policyholderId"))
        .setLineOfBusiness(com.tidewater.claims.common.LineOfBusiness.valueOf(claim.ref("lineOfBusiness")))
        .build())
    .setScore(score.value)
    .setSignals(score.signals)
    .build()
