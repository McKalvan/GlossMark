package com.tidewater.claims.fraud

import io.confluent.kafka.schemaregistry.testutil.MockSchemaRegistry
import io.confluent.kafka.streams.serdes.avro.GenericAvroSerde
import io.confluent.kafka.streams.serdes.avro.SpecificAvroSerde
import org.apache.avro.Schema
import org.apache.avro.generic.GenericData
import org.apache.avro.generic.GenericRecord
import org.apache.avro.specific.SpecificRecord
import org.apache.kafka.common.serialization.Serdes
import org.apache.kafka.streams.TopologyTestDriver
import java.io.File
import java.util.Properties
import kotlin.test.AfterTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

// @gloss verifies .ScreeningTopology conf=0.55
class ScreeningTopologyTest {
    private val registry = mapOf("schema.registry.url" to "mock://screening-test")
    private val filedSerde = GenericAvroSerde().apply { configure(registry, false) }
    private val alertSerde = SpecificAvroSerde<SpecificRecord>().apply { configure(registry, false) }
    private val driver = TopologyTestDriver(
        screeningTopology(filedSerde, alertSerde),
        Properties().apply { put("application.id", "screening-test"); put("bootstrap.servers", "unused:9092") },
    )
    private val input = driver.createInputTopic(FILED_TOPIC, Serdes.String().serializer(), filedSerde.serializer())
    private val alerts = driver.createOutputTopic(ALERTS_TOPIC, Serdes.String().deserializer(), alertSerde.deserializer())

    private val parser = Schema.Parser().apply {
        parse(File("../../schemas/Money.avsc"))
        parse(File("../../schemas/ClaimRef.avsc"))
    }
    private val motorSchema = parser.parse(File("../../schemas/MotorClaimFiled.avsc"))

    private fun motor(claimId: String, holder: String, lossMinor: Long, policeReport: String?): GenericRecord =
        GenericData.Record(motorSchema).apply {
            put("eventId", "e-$claimId")
            put("occurredAt", 1_790_000_000_000L)
            put("claim", GenericData.Record(motorSchema.getField("claim").schema()).apply {
                put("claimId", claimId); put("policyNumber", "MTR-1"); put("policyholderId", holder)
                put("lineOfBusiness", GenericData.EnumSymbol(schema.getField("lineOfBusiness").schema(), "MOTOR"))
            })
            put("lossDate", 20_700)
            put("estimatedLoss", GenericData.Record(motorSchema.getField("estimatedLoss").schema()).apply {
                put("amountMinor", lossMinor); put("currency", "EUR")
            })
            put("vehicleVin", "VIN"); put("thirdPartyInvolved", false); put("policeReportNumber", policeReport)
            put("incidentKind", GenericData.EnumSymbol(motorSchema.getField("incidentKind").schema(), "THEFT"))
            put("evidenceIds", emptyList<String>())
        }

    @AfterTest
    fun close() {
        driver.close()
        MockSchemaRegistry.dropScope("screening-test")
    }

    @Test
    fun `a third claim in the window with high value and no report raises FraudSuspected`() {
        input.pipeInput("c-1", motor("c-1", "h-9", 10_000, "PR-1"))
        input.pipeInput("c-2", motor("c-2", "h-9", 10_000, "PR-2"))
        input.pipeInput("c-3", motor("c-3", "h-9", 900_000, null))

        val raised = alerts.readKeyValuesToList()
        assertEquals(listOf("c-3"), raised.map { it.key })
        assertTrue(raised.single().value.toString().contains("HIGH_VALUE_NO_REPORT"))
    }
}
