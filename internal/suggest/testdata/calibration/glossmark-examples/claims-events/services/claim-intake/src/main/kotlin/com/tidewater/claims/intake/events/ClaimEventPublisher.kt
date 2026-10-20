package com.tidewater.claims.intake.events

import com.tidewater.claims.events.ClaimWithdrawn
import com.tidewater.claims.events.WithdrawalReason
import com.tidewater.claims.common.ClaimRef
import com.tidewater.claims.common.LineOfBusiness
import com.tidewater.claims.intake.domain.Claim
import com.tidewater.claims.intake.domain.ClaimHeader
import io.confluent.kafka.serializers.KafkaAvroSerializer
import io.confluent.kafka.serializers.KafkaAvroSerializerConfig
import io.confluent.kafka.serializers.subject.TopicRecordNameStrategy
import io.ktor.server.config.ApplicationConfig
import org.apache.kafka.clients.producer.KafkaProducer
import org.apache.kafka.clients.producer.Producer
import org.apache.kafka.clients.producer.ProducerConfig
import org.apache.kafka.clients.producer.ProducerRecord
import org.apache.kafka.common.serialization.StringSerializer
import org.apache.avro.specific.SpecificRecord
import java.time.Instant
import java.util.UUID

/**
 * Publishes claim events. Idempotent and transactional, so a retried request never
 * produces a second ClaimFiled for the same claim.
 */
class ClaimEventPublisher(
    private val producer: Producer<String, SpecificRecord>,
    private val filedTopic: String,
    private val lifecycleTopic: String,
) : AutoCloseable {
    init {
        producer.initTransactions()
    }

    // @gloss publishes claim:MotorClaimFiled, claim:FloodClaimFiled, claim:FireClaimFiled conf=0.9
    fun publishFiled(claim: Claim) = send(filedTopic, claim.id, claim.toFiledEvent())

    // @gloss publishes claim:ClaimWithdrawn conf=0.9
    fun publishWithdrawn(claim: ClaimHeader, reason: String, at: Instant) = send(
        lifecycleTopic,
        claim.id,
        ClaimWithdrawn.newBuilder()
            .setEventId(UUID.randomUUID().toString())
            .setOccurredAt(at)
            .setClaim(ClaimRef(claim.id.toString(), claim.policyNumber, claim.policyholderId.toString(), LineOfBusiness.valueOf(claim.lineOfBusiness)))
            .setReason(WithdrawalReason.valueOf(reason))
            .build(),
    )

    private fun send(topic: String, key: UUID, value: SpecificRecord) {
        producer.beginTransaction()
        try {
            producer.send(ProducerRecord(topic, key.toString(), value)).get()
            producer.commitTransaction()
        } catch (e: Exception) {
            producer.abortTransaction()
            throw e
        }
    }

    override fun close() = producer.close()

    companion object {
        fun fromConfig(config: ApplicationConfig): ClaimEventPublisher {
            val props = mapOf(
                ProducerConfig.BOOTSTRAP_SERVERS_CONFIG to config.property("bootstrap").getString(),
                ProducerConfig.KEY_SERIALIZER_CLASS_CONFIG to StringSerializer::class.java,
                ProducerConfig.VALUE_SERIALIZER_CLASS_CONFIG to KafkaAvroSerializer::class.java,
                ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG to true,
                ProducerConfig.ACKS_CONFIG to "all",
                ProducerConfig.TRANSACTIONAL_ID_CONFIG to config.property("transactionalIdPrefix").getString() +
                    (System.getenv("HOSTNAME") ?: UUID.randomUUID().toString()),
                KafkaAvroSerializerConfig.SCHEMA_REGISTRY_URL_CONFIG to config.property("schemaRegistry").getString(),
                KafkaAvroSerializerConfig.AUTO_REGISTER_SCHEMAS to false,
                KafkaAvroSerializerConfig.VALUE_SUBJECT_NAME_STRATEGY to TopicRecordNameStrategy::class.java.name,
            )
            return ClaimEventPublisher(
                KafkaProducer(props),
                config.property("filedTopic").getString(),
                config.property("lifecycleTopic").getString(),
            )
        }
    }
}
