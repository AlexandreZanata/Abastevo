package com.anpfuel.application.usecase.profile

import com.anpfuel.domain.profile.ManagementAction
import com.anpfuel.domain.profile.ManagementDecision
import com.anpfuel.domain.profile.ManagementCapabilityRule
import com.anpfuel.domain.profile.ManagementGrant
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P32-T03 — Managed-action orchestration RED→GREEN.
 */
class ManagedActionUseCaseTest {

    private class FakeManagedGateway : ManagedActionGateway {
        val sent = mutableListOf<ManagementAction>()
        var revokedNow = false

        override suspend fun send(action: ManagementAction): Boolean {
            if (revokedNow) return false
            sent.add(action)
            return true
        }
    }

    private val grant = ManagementGrant(
        role = "manager",
        scopes = setOf("edit_hours", "reply_reviews", "invite_manager", "contest_access", "reverify"),
        freshAuth = true,
        revoked = false,
        suspended = false,
        operatorStale = false,
    )

    @Test
    fun `denied capability never reaches gateway`() = runTest {
        val gateway = FakeManagedGateway()
        val outcome = PerformManagedActionUseCase(gateway).invoke(
            grant.copy(revoked = true),
            ManagementAction.EditBusiness,
        )
        assertEquals(ManagedActionOutcome.Denied(ManagementDecision.DeniedRevoked), outcome)
        assertTrue(gateway.sent.isEmpty())
    }

    @Test
    fun `server refusal recovers without replay`() = runTest {
        val gateway = FakeManagedGateway().also { it.revokedNow = true }
        val outcome = PerformManagedActionUseCase(gateway).invoke(grant, ManagementAction.ReplyReview)
        assertEquals(ManagedActionOutcome.ServerRefused, outcome)
        assertTrue(gateway.sent.isEmpty())
    }

    @Test
    fun `allowed action sent once`() = runTest {
        val gateway = FakeManagedGateway()
        val outcome = PerformManagedActionUseCase(gateway).invoke(grant, ManagementAction.InviteManager)
        assertEquals(ManagedActionOutcome.SentPendingServer, outcome)
        assertEquals(listOf(ManagementAction.InviteManager), gateway.sent)
    }
}
