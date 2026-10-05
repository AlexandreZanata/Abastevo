package com.anpfuel.domain.profile

/**
 * P32-T03 — Server-authoritative management capabilities.
 *
 * The UI never grants: [ManagementDecision.AllowedRequiresServer] means
 * the action may be sent, and only the server decision applies. All other
 * outcomes block before I/O so stale privilege is never replayed.
 */
enum class ManagementAction {
    EditBusiness,
    ReplyReview,
    InviteManager,
    ContestAccess,
    RequestReverification,
}

data class ManagementGrant(
    val role: String,
    val scopes: Set<String>,
    val freshAuth: Boolean,
    val revoked: Boolean,
    val suspended: Boolean,
    val operatorStale: Boolean,
)

sealed interface ManagementDecision {
    data object AllowedRequiresServer : ManagementDecision

    data object DeniedRevoked : ManagementDecision

    data object DeniedSuspended : ManagementDecision

    data object DeniedStaleOperator : ManagementDecision

    data object DeniedFreshAuth : ManagementDecision

    data object DeniedScope : ManagementDecision

    data object DeniedInvalid : ManagementDecision
}

object ManagementCapabilityRule {

    const val MAX_REPLY_SCALARS = 280

    private val scopeFor = mapOf(
        ManagementAction.EditBusiness to "edit_hours",
        ManagementAction.ReplyReview to "reply_reviews",
        ManagementAction.InviteManager to "invite_manager",
        ManagementAction.ContestAccess to "contest_access",
        ManagementAction.RequestReverification to "reverify",
    )

    fun can(grant: ManagementGrant, action: ManagementAction): ManagementDecision {
        if (grant.revoked) return ManagementDecision.DeniedRevoked
        if (grant.suspended) return ManagementDecision.DeniedSuspended
        if (grant.operatorStale) return ManagementDecision.DeniedStaleOperator
        if ((action == ManagementAction.InviteManager || action == ManagementAction.ContestAccess) && !grant.freshAuth) {
            return ManagementDecision.DeniedFreshAuth
        }
        val required = scopeFor[action] ?: return ManagementDecision.DeniedInvalid
        if (!grant.scopes.contains(required)) return ManagementDecision.DeniedScope
        return ManagementDecision.AllowedRequiresServer
    }

    fun checkReplyText(text: String): ManagementDecision {
        if (text.isBlank()) return ManagementDecision.DeniedInvalid
        if (text.codePointCount(0, text.length) > MAX_REPLY_SCALARS) return ManagementDecision.DeniedInvalid
        return ManagementDecision.AllowedRequiresServer
    }
}
