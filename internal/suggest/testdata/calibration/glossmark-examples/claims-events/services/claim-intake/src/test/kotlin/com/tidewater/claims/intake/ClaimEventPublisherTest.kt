package com.tidewater.claims.intake

import com.tidewater.claims.events.FloodClaimFiled
import com.tidewater.claims.events.MotorClaimFiled
import com.tidewater.claims.intake.domain.ClaimStatus
import com.tidewater.claims.intake.domain.FloodClaim
import com.tidewater.claims.intake.domain.Money
import com.tidewater.claims.intake.domain.MotorClaim
import com.tidewater.claims.intake.events.ClaimEventPublisher
import org.apache.avro.specific.SpecificRecord
import org.apache.kafka.clients.producer.MockProducer
import org.apache.kafka.common.serialization.Serializer
import org.apache.kafka.common.serialization.StringSerializer
import java.time.Instant
import java.time.LocalDate
import java.util.UUID
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertIs
import kotlin.test.assertTrue

// @gloss verifies claim:ClaimFiled conf=0.9
class ClaimEventPublisherTest {
    private val noopAvro = Serializer<SpecificRecord> { _, _ -> ByteArray(0) }
    private val producer = MockProducer(true, StringSerializer(), noopAvro)
    private val publisher = ClaimEventPublisher(producer, "claims.filed.v1", "claims.lifecycle.v1")

    private val holder = UUID.randomUUID()
    private val filedAt = Instant.parse("2026-10-01T09:00:00Z")

    @Test
    fun `a motor claim is published as MotorClaimFiled keyed by claim id`() {
        val claim = MotorClaim(UUID.randomUUID(), "MTR-100234", holder, LocalDate.of(2026, 9, 28), Money(184_000, "EUR"),
            filedAt, ClaimStatus.FILED, listOf("ev-1"), "WVWZZZ1JZXW000001", "COLLISION", "PR-77", true)
        publisher.publishFiled(claim)

        val sent = producer.history().single()
        assertEquals("claims.filed.v1", sent.topic())
        assertEquals(claim.id.toString(), sent.key())
        assertIs<MotorClaimFiled>(sent.value())
        assertTrue(producer.transactionCommitted())
    }

    @Test
    fun `a flood claim is published as FloodClaimFiled with the flood source`() {
        val claim = FloodClaim(UUID.randomUUID(), "HOM-55120", holder, LocalDate.of(2026, 9, 20), Money(1_250_000, "EUR"),
            filedAt, ClaimStatus.FILED, emptyList(), "PROP-9", 40, "RIVER", true)
        publisher.publishFiled(claim)

        val event = assertIs<FloodClaimFiled>(producer.history().single().value())
        assertEquals("RIVER", event.floodSource.name)
    }
}
