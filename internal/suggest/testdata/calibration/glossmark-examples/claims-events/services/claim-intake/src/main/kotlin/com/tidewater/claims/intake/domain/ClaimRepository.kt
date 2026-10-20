package com.tidewater.claims.intake.domain

import org.jetbrains.exposed.sql.Database
import org.jetbrains.exposed.sql.SqlExpressionBuilder.eq
import org.jetbrains.exposed.sql.and
import org.jetbrains.exposed.sql.ResultRow
import org.jetbrains.exposed.sql.Table
import org.jetbrains.exposed.sql.insert
import org.jetbrains.exposed.sql.javatime.date
import org.jetbrains.exposed.sql.javatime.timestamp
import org.jetbrains.exposed.sql.selectAll
import org.jetbrains.exposed.sql.transactions.transaction
import org.jetbrains.exposed.sql.update
import java.util.UUID

// @gloss represents claim:Claim conf=0.85
object ClaimsTable : Table("claims") {
    val id = uuid("id")
    val lineOfBusiness = varchar("line_of_business", 16)
    val kind = varchar("kind", 16)
    val policyNumber = varchar("policy_number", 32)
    val policyholderId = uuid("policyholder_id")
    val lossDate = date("loss_date")
    val estimatedLossMinor = long("estimated_loss_minor")
    val currency = varchar("currency", 3)
    val status = varchar("status", 16)
    val filedAt = timestamp("filed_at")
    val details = text("details_json")
    override val primaryKey = PrimaryKey(id)
}

/** What every event about a claim carries, read without loading the line-of-business details. */
data class ClaimHeader(val id: UUID, val policyNumber: String, val policyholderId: UUID, val lineOfBusiness: String)

class ClaimRepository private constructor(private val db: Database) {
    fun insert(claim: Claim, detailsJson: String) = transaction(db) {
        ClaimsTable.insert {
            it[id] = claim.id
            it[lineOfBusiness] = if (claim is MotorClaim) "MOTOR" else "HOME"
            it[kind] = claim::class.simpleName!!.removeSuffix("Claim").uppercase()
            it[policyNumber] = claim.policyNumber
            it[policyholderId] = claim.policyholderId
            it[lossDate] = claim.lossDate
            it[estimatedLossMinor] = claim.estimatedLoss.amountMinor
            it[currency] = claim.estimatedLoss.currency
            it[status] = claim.status.name
            it[filedAt] = claim.filedAt
            it[details] = detailsJson
        }
    }

    fun status(id: UUID): ClaimStatus? = transaction(db) {
        ClaimsTable.selectAll().where { ClaimsTable.id eq id }
            .map { row: ResultRow -> ClaimStatus.valueOf(row[ClaimsTable.status]) }
            .singleOrNull()
    }

    fun header(id: UUID): ClaimHeader = transaction(db) {
        ClaimsTable.selectAll().where { ClaimsTable.id eq id }.single().let { row ->
            ClaimHeader(row[ClaimsTable.id], row[ClaimsTable.policyNumber], row[ClaimsTable.policyholderId], row[ClaimsTable.lineOfBusiness])
        }
    }

    fun markWithdrawn(id: UUID): Boolean = transaction(db) {
        ClaimsTable.update({ (ClaimsTable.id eq id) and (ClaimsTable.status eq ClaimStatus.FILED.name) }) {
            it[status] = ClaimStatus.WITHDRAWN.name
        } == 1
    }

    companion object {
        fun connect(url: String) = ClaimRepository(Database.connect(url, driver = "org.postgresql.Driver"))
    }
}
