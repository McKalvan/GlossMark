package com.tidewater.claims.intake

import com.tidewater.claims.intake.api.claimRoutes
import com.tidewater.claims.intake.events.ClaimEventPublisher
import com.tidewater.claims.intake.policy.PolicyClient
import com.tidewater.claims.intake.withdrawal.WithdrawalService
import com.tidewater.claims.intake.domain.ClaimRepository
import io.ktor.http.HttpStatusCode
import io.ktor.serialization.kotlinx.json.json
import io.ktor.server.application.Application
import io.ktor.server.application.install
import io.ktor.server.plugins.contentnegotiation.ContentNegotiation
import io.ktor.server.plugins.statuspages.StatusPages
import io.ktor.server.response.respond
import io.ktor.server.routing.routing
import java.time.Clock

fun main(args: Array<String>) = io.ktor.server.netty.EngineMain.main(args)

fun Application.module() {
    val config = environment.config
    val publisher = ClaimEventPublisher.fromConfig(config.config("kafka"))
    val policies = PolicyClient(config.property("policyAdmin.baseUrl").getString())
    val claims = ClaimRepository.connect(config.property("database.url").getString())
    val withdrawals = WithdrawalService(claims, publisher, Clock.systemUTC())

    install(ContentNegotiation) { json() }
    install(StatusPages) {
        exception<IllegalArgumentException> { call, cause ->
            call.respond(HttpStatusCode.UnprocessableEntity, mapOf("error" to cause.message))
        }
    }
    routing {
        claimRoutes(claims, policies, publisher, withdrawals, Clock.systemUTC())
    }
    environment.monitor.subscribe(io.ktor.server.application.ApplicationStopped) { publisher.close() }
}
