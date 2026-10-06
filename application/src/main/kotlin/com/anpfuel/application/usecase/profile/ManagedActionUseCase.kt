package com.anpfuel.application.usecase.profile

import com.anpfuel.domain.profile.ManagementAction
import com.anpfuel.domain.profile.ManagementCapabilityRule
import com.anpfuel.domain.profile.ManagementDecision
import com.anpfuel.domain.profile.ManagementGrant

/**
 * P32-T03 — Managed-action orchestration.
 *
 * Capability is checked before I/O and the server decision is final:
 * `SentPendingServer` never means approved. Server refusals recover
 * without replaying stale privilege.
 */
interface ManagedActionGateway {
    suspend fun send(action: ManagementAction): Boolean
}

sealed interface ManagedActionOutcome {
    data class Denied(val reason: ManagementDecision) : ManagedActionOutcome

    data object SentPendingServer : ManagedActionOutcome

    data object ServerRefused : ManagedActionOutcome
}

class PerformManagedActionUseCase(
    private val gateway: ManagedActionGateway,
) {
    suspend operator fun invoke(grant: ManagementGrant, action: ManagementAction): ManagedActionOutcome {
        val decision = ManagementCapabilityRule.can(grant, action)
        if (decision != ManagementDecision.AllowedRequiresServer) {
            return ManagedActionOutcome.Denied(decision)
        }
        return if (gateway.send(action)) ManagedActionOutcome.SentPendingServer
        else ManagedActionOutcome.ServerRefused
    }
}
