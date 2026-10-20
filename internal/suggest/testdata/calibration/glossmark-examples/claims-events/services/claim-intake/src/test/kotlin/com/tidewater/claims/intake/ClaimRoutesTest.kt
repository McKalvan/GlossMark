package com.tidewater.claims.intake

import com.tidewater.claims.intake.api.FileFireClaim
import com.tidewater.claims.intake.api.FileMotorClaim
import com.tidewater.claims.intake.api.MoneyDto
import com.tidewater.claims.intake.api.fileClaim
import com.tidewater.claims.intake.domain.FireClaim
import com.tidewater.claims.intake.domain.HomeClaim
import com.tidewater.claims.intake.policy.PolicyClient
import com.tidewater.claims.intake.policy.PolicySummary
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import java.time.Clock
import java.time.Instant
import java.time.ZoneOffset
import java.util.UUID
import kotlin.test.Test
import kotlin.test.assertFailsWith
import kotlin.test.assertIs

// @gloss verifies .FilingHandler conf=0.9
class ClaimRoutesTest {
    private val clock = Clock.fixed(Instant.parse("2026-10-01T09:00:00Z"), ZoneOffset.UTC)
    private val holder = UUID.randomUUID().toString()
    private val policies = mockk<PolicyClient> {
        coEvery { find("HOM-1") } returns PolicySummary("HOM-1", holder, "HOME", "2026-01-01", "2026-12-31", listOf("FIRE"))
        coEvery { find("MTR-1") } returns PolicySummary("MTR-1", holder, "MOTOR", "2026-01-01", "2026-12-31", emptyList())
        coEvery { find("NONE") } returns null
    }

    @Test
    fun `a fire claim on a home policy becomes a FireClaim`() = runTest {
        val claim = fileClaim(FileFireClaim("HOM-1", "2026-09-30", MoneyDto(500_000, "EUR"), emptyList(), "PROP-3", true), policies, clock)
        assertIs<FireClaim>(claim)
        assertIs<HomeClaim>(claim)
    }

    @Test
    fun `a motor claim on a home policy is refused`() = runTest {
        assertFailsWith<IllegalArgumentException> {
            fileClaim(FileMotorClaim("HOM-1", "2026-09-30", MoneyDto(10_000, "EUR"), emptyList(), "VIN", "GLASS", null, false), policies, clock)
        }
    }

    @Test
    fun `an unknown policy is refused`() = runTest {
        assertFailsWith<IllegalArgumentException> {
            fileClaim(FileFireClaim("NONE", "2026-09-30", MoneyDto(1, "EUR"), emptyList(), "P", false), policies, clock)
        }
    }
}
