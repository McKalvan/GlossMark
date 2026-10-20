plugins {
    kotlin("jvm") version "2.0.21"
    kotlin("plugin.serialization") version "2.0.21"
    id("io.ktor.plugin") version "2.3.12"
    id("com.github.davidmc24.gradle.plugin.avro") version "1.9.1"
}

group = "com.tidewater.claims"
version = "1.8.0"

application {
    mainClass.set("com.tidewater.claims.intake.ApplicationKt")
}

repositories {
    mavenCentral()
    maven("https://packages.confluent.io/maven/")
}

avro {
    fieldVisibility.set("PRIVATE")
    stringType.set("String")
}

// Generate Java classes from the shared schema directory, not a copy.
tasks.named<com.github.davidmc24.gradle.plugin.avro.GenerateAvroJavaTask>("generateAvroJava") {
    source(rootDir.resolve("../../schemas"))
}

dependencies {
    implementation("io.ktor:ktor-server-netty")
    implementation("io.ktor:ktor-server-content-negotiation")
    implementation("io.ktor:ktor-serialization-kotlinx-json")
    implementation("io.ktor:ktor-server-status-pages")
    implementation("io.ktor:ktor-client-cio")
    implementation("io.ktor:ktor-client-content-negotiation")
    implementation("org.apache.kafka:kafka-clients:3.8.0")
    implementation("io.confluent:kafka-avro-serializer:7.7.1")
    implementation("org.apache.avro:avro:1.12.0")
    implementation("org.jetbrains.exposed:exposed-jdbc:0.55.0")
    implementation("org.postgresql:postgresql:42.7.4")
    implementation("ch.qos.logback:logback-classic:1.5.8")

    testImplementation("io.ktor:ktor-server-test-host")
    testImplementation("org.jetbrains.kotlin:kotlin-test-junit5")
    testImplementation("org.apache.kafka:kafka-clients:3.8.0:test")
    testImplementation("io.mockk:mockk:1.13.12")
}

kotlin {
    jvmToolchain(21)
}

tasks.test {
    useJUnitPlatform()
}
