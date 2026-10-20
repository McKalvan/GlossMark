package com.tidewater.claims.fraud

import io.confluent.kafka.streams.serdes.avro.GenericAvroSerde
import io.confluent.kafka.streams.serdes.avro.SpecificAvroSerde
import org.apache.avro.specific.SpecificRecord
import org.apache.kafka.streams.KafkaStreams
import org.apache.kafka.streams.StreamsConfig
import java.util.Properties
import java.util.concurrent.CountDownLatch

fun main() {
    val props = Properties().apply {
        FraudScreeningApp::class.java.getResourceAsStream("/streams.properties").use { load(it) }
        put(StreamsConfig.BOOTSTRAP_SERVERS_CONFIG, System.getenv("KAFKA_BOOTSTRAP"))
    }
    val registry = mapOf(
        "schema.registry.url" to System.getenv("SCHEMA_REGISTRY_URL"),
        "auto.register.schemas" to false,
        "value.subject.name.strategy" to "io.confluent.kafka.serializers.subject.TopicRecordNameStrategy",
    )
    val filedSerde = GenericAvroSerde().apply { configure(registry, false) }
    val alertSerde = SpecificAvroSerde<SpecificRecord>().apply { configure(registry, false) }

    val streams = KafkaStreams(screeningTopology(filedSerde, alertSerde), props)
    val stopped = CountDownLatch(1)
    Runtime.getRuntime().addShutdownHook(Thread {
        streams.close()
        stopped.countDown()
    })
    streams.start()
    stopped.await()
}

private object FraudScreeningApp
