plugins {
    kotlin("jvm") version "2.0.21"
    application
    id("com.github.davidmc24.gradle.plugin.avro") version "1.9.1"
}

group = "com.tidewater.claims"
version = "2.1.0"

application {
    mainClass.set("com.tidewater.claims.fraud.FraudScreeningAppKt")
}

repositories {
    mavenCentral()
    maven("https://packages.confluent.io/maven/")
}

tasks.named<com.github.davidmc24.gradle.plugin.avro.GenerateAvroJavaTask>("generateAvroJava") {
    source(rootDir.resolve("../../schemas"))
}

dependencies {
    implementation("org.apache.kafka:kafka-streams:3.8.0")
    implementation("io.confluent:kafka-streams-avro-serde:7.7.1")
    implementation("org.apache.avro:avro:1.12.0")
    implementation("ch.qos.logback:logback-classic:1.5.8")

    testImplementation("org.apache.kafka:kafka-streams-test-utils:3.8.0")
    testImplementation("org.jetbrains.kotlin:kotlin-test-junit5")
}

kotlin { jvmToolchain(21) }

tasks.test { useJUnitPlatform() }
