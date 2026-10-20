package com.tidewater.claims.intake.policy

import io.ktor.client.HttpClient
import io.ktor.client.call.body
import io.ktor.client.engine.cio.CIO
import io.ktor.client.plugins.HttpTimeout
import io.ktor.client.plugins.contentnegotiation.ContentNegotiation
import io.ktor.client.request.get
import io.ktor.http.HttpStatusCode
import io.ktor.serialization.kotlinx.json.json
import kotlinx.serialization.Serializable
import java.time.LocalDate

@Serializable
data class PolicySummary(
    val policyNumber: String,
    val policyholderId: String,
    val lineOfBusiness: String,
    val inForceFrom: String,
    val inForceTo: String,
    val coveredPerils: List<String>,
) {
    fun coversOn(date: LocalDate) =
        !date.isBefore(LocalDate.parse(inForceFrom)) && !date.isAfter(LocalDate.parse(inForceTo))
}

/** Reads policies from policy administration. Intake never writes to it. */
// @gloss consumes claim:Policy conf=0.85
class PolicyClient(private val baseUrl: String) {
    private val http = HttpClient(CIO) {
        install(ContentNegotiation) { json() }
        install(HttpTimeout) { requestTimeoutMillis = 1500 }
    }

    suspend fun find(policyNumber: String): PolicySummary? {
        val response = http.get("$baseUrl/policies/$policyNumber")
        return if (response.status == HttpStatusCode.NotFound) null else response.body()
    }
}
