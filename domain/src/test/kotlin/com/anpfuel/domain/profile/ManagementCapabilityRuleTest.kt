package com.anpfuel.domain.profile

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

/**
 * P32-T03 — Management capability RED→GREEN.
 *
 * Locks: server-authoritative grants only (no UI-only gates); revoked or
 * suspended roles deny mid-edit; stale operator data denies; missing
 * fresh-auth denies sensitive actions; unknown actions deny; official
 * replies stay within the 280-scalar transport boundary.
 */
class ManagementCapabilityRuleTest {

    private fun grant(
        role: String = "manager",
        scopes: Set<String> = setOf("edit_hours", "reply_reviews"),
        freshAuth: Boolean = true,
        revoked: Boolean = false,
        suspended: Boolean = false,
        operatorStale: Boolean = false,
    ) = ManagementGrant(
        role = role,
        scopes = scopes,
        freshAuth = freshAuth,
        revoked = revoked,
        suspended = suspended,
        operatorStale = operatorStale,
    )

    @Test
    fun `revoked role denies mid-edit`() {
        assertEquals(
            ManagementDecision.DeniedRevoked,
            ManagementCapabilityRule.can(grant(revoked = true), ManagementAction.EditBusiness),
        )
    }

    @Test
    fun `suspended role denies replies`() {
        assertEquals(
            ManagementDecision.DeniedSuspended,
            ManagementCapabilityRule.can(grant(suspended = true), ManagementAction.ReplyReview),
        )
    }

    @Test
    fun `stale operator denies edits`() {
        assertEquals(
            ManagementDecision.DeniedStaleOperator,
            ManagementCapabilityRule.can(grant(operatorStale = true), ManagementAction.EditBusiness),
        )
    }

    @Test
    fun `invites require fresh auth`() {
        assertEquals(
            ManagementDecision.DeniedFreshAuth,
            ManagementCapabilityRule.can(grant(freshAuth = false), ManagementAction.InviteManager),
        )
    }

    @Test
    fun `out-of-scope action denies`() {
        assertEquals(
            ManagementDecision.DeniedScope,
            ManagementCapabilityRule.can(grant(scopes = setOf("edit_hours")), ManagementAction.ReplyReview),
        )
    }

    @Test
    fun `allowed edit carries no optimistic approval`() {
        val decision = ManagementCapabilityRule.can(grant(), ManagementAction.EditBusiness)
        assertEquals(ManagementDecision.AllowedRequiresServer, decision)
    }

    @Test
    fun `overlong official reply refused`() {
        val long = "x".repeat(281)
        assertEquals(
            ManagementDecision.DeniedInvalid,
            ManagementCapabilityRule.checkReplyText(long),
        )
        assertEquals(
            ManagementDecision.AllowedRequiresServer,
            ManagementCapabilityRule.checkReplyText("Obrigado pela visita!"),
        )
    }
}
