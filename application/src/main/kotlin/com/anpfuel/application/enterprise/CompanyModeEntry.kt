package com.anpfuel.application.enterprise

/**
 * Device-local hint that the signed account had an active company membership
 * at its last server answer. It only decides whether the Perfil entry is open;
 * it is never authority: the context screen re-reads membership from the server.
 */
interface EnterpriseMembershipHints {
    /** Null when this account was never checked on this device. */
    fun linked(accountId: String, origin: String): Boolean?
    fun record(accountId: String, origin: String, linked: Boolean)
}

/**
 * B-BR-EMP-ENTRY-01 — the Perfil "Company mode" entry is open only for a
 * signed account with a company link (a server-confirmed company list, an
 * accepted invitation with an active membership, or the remembered hint of
 * either). An active company context always keeps the entry open, so the way
 * back to personal mode never disappears.
 */
object CompanyModeEntryRule {

    fun enabled(binding: EnterpriseAccessBinding?, hint: Boolean?, state: EnterpriseContextState): Boolean {
        if (binding == null) return false
        if (state.selected != null) return true
        return if (state.catalogConfirmed) state.contexts.isNotEmpty() else hint == true
    }

    /** The link a confirmed, idle company list proves; null while it proves nothing. */
    fun linkToRecord(state: EnterpriseContextState): Boolean? =
        if (state.catalogConfirmed && !state.busy) state.contexts.isNotEmpty() else null

    /** An accepted invitation links the account once its membership is active. */
    fun linkedBy(state: EnterpriseInvitationState): Boolean =
        state.stage == EnterpriseInvitationStage.RESULT && state.acknowledgement?.active == true

    /**
     * Perfil may read the company list itself only when the link is unknown and
     * no company context is in use, so it never disturbs an active session.
     */
    fun shouldProbe(binding: EnterpriseAccessBinding?, hint: Boolean?, state: EnterpriseContextState): Boolean =
        binding != null && hint == null && state.selected == null && !state.busy
}
