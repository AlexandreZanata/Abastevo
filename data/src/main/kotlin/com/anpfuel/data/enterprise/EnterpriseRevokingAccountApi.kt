package com.anpfuel.data.enterprise

import com.anpfuel.application.portable.AuthAccountApi
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.portable.PortableAuth

/** Only an authoritative denial for the still-current requested family triggers erasure. */
class EnterpriseRevokingAccountApi(
    private val delegate: AuthAccountApi,
    private val sessions: AuthSessionStore,
    private val origin: String,
    private val erasure: EnterpriseVaultErasure,
) : AuthAccountApi by delegate {
    override fun refresh(familyId: String, refreshToken: String): AuthApiResult<PortableAuth.Session> {
        val before = sessions.load()?.takeIf { it.familyId == familyId && it.refreshToken == refreshToken }
        val result = delegate.refresh(familyId, refreshToken)
        if (before != null && result is AuthApiResult.Err &&
            result.verdict in setOf(PortableAuth.Verdict.SESSION_REVOKED, PortableAuth.Verdict.SESSION_REUSE) &&
            sessions.load() == before
        ) {
            // Keep the backend verdict so AuthFlow also clears account authority.
            // cleanupRequired remains visible even when physical cleanup fails.
            erasure.accountRevoked(before.accountId, origin)
        }
        return result
    }
}
