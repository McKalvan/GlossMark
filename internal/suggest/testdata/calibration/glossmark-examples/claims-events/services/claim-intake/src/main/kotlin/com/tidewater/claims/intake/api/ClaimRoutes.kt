package com.tidewater.claims.intake.api

import com.tidewater.claims.intake.domain.Claim
import com.tidewater.claims.intake.domain.ClaimRepository
import com.tidewater.claims.intake.domain.ClaimStatus
import com.tidewater.claims.intake.domain.FireClaim
import com.tidewater.claims.intake.domain.FloodClaim
import com.tidewater.claims.intake.domain.Money
import com.tidewater.claims.intake.domain.MotorClaim
import com.tidewater.claims.intake.domain.requireWithinFilingWindow
import com.tidewater.claims.intake.events.ClaimEventPublisher
import com.tidewater.claims.intake.policy.PolicyClient
import com.tidewater.claims.intake.withdrawal.WithdrawalService
import io.ktor.http.HttpStatusCode
import io.ktor.server.request.receive
import io.ktor.server.response.respond
import io.ktor.server.routing.Route
import io.ktor.server.routing.post
import io.ktor.server.routing.route
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import java.time.Clock
import java.time.LocalDate
import java.util.UUID

fun Route.claimRoutes(
    claims: ClaimRepository,
    policies: PolicyClient,
    publisher: ClaimEventPublisher,
    withdrawals: WithdrawalService,
    clock: Clock,
) {
    route("/v1/claims") {
        post {
            val request = call.receive<FileClaimRequest>()
            val claim = fileClaim(request, policies, clock)
            claims.insert(claim, Json.encodeToString(request))
            publisher.publishFiled(claim)
            call.respond(HttpStatusCode.Accepted, ClaimAccepted(claim.id.toString(), claim.status.name))
        }

        post("/{id}/withdrawal") {
            val id = UUID.fromString(call.parameters["id"])
            val request = call.receive<WithdrawClaimRequest>()
            when (withdrawals.withdraw(id, request.reason)) {
                WithdrawalService.Outcome.WITHDRAWN -> call.respond(HttpStatusCode.Accepted, ClaimAccepted(id.toString(), "WITHDRAWN"))
                WithdrawalService.Outcome.NOT_FOUND -> call.respond(HttpStatusCode.NotFound)
                WithdrawalService.Outcome.ALREADY_DECIDED -> call.respond(HttpStatusCode.Conflict)
            }
        }
    }
}

// @gloss implements .FilingHandler conf=0.9
// @gloss produces claim:MotorClaim, claim:FloodClaim, claim:FireClaim conf=0.85
suspend fun fileClaim(request: FileClaimRequest, policies: PolicyClient, clock: Clock): Claim {
    val lossDate = LocalDate.parse(request.lossDate)
    val policy = requireNotNull(policies.find(request.policyNumber)) { "unknown policy ${request.policyNumber}" }
    require(policy.coversOn(lossDate)) { "policy was not in force on $lossDate" }
    requireWithinFilingWindow(lossDate, clock)

    val id = UUID.randomUUID()
    val holder = UUID.fromString(policy.policyholderId)
    val loss = Money(request.estimatedLoss.amountMinor, request.estimatedLoss.currency)
    val now = clock.instant()
    return when (request) {
        is FileMotorClaim -> {
            require(policy.lineOfBusiness == "MOTOR") { "a motor claim needs a motor policy" }
            MotorClaim(id, request.policyNumber, holder, lossDate, loss, now, ClaimStatus.FILED, request.evidenceIds,
                request.vehicleVin, request.incidentKind, request.policeReportNumber, request.thirdPartyInvolved)
        }
        is FileFloodClaim -> {
            require("FLOOD" in policy.coveredPerils) { "policy does not cover flood" }
            FloodClaim(id, request.policyNumber, holder, lossDate, loss, now, ClaimStatus.FILED, request.evidenceIds,
                request.propertyId, request.waterDepthCm, request.floodSource, request.inFloodZone)
        }
        is FileFireClaim -> {
            require("FIRE" in policy.coveredPerils) { "policy does not cover fire" }
            FireClaim(id, request.policyNumber, holder, lossDate, loss, now, ClaimStatus.FILED, request.evidenceIds,
                request.propertyId, request.fireServiceAttended, request.fireReportNumber)
        }
    }
}
