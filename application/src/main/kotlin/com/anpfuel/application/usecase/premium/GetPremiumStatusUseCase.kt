package com.anpfuel.application.usecase.premium

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.premium.PremiumStatusException
import com.anpfuel.domain.premium.PremiumStatusSnapshot
import com.anpfuel.domain.repository.PremiumStatusGateway

/**
 * MEX-16 verified Premium status for the signed-in owner.
 *
 * The local session is only a hint: the backend revalidates the family
 * proof and derives the status from its own purchase snapshot. A
 * missing session refuses before any private call, and the returned
 * account id must still match the session that initiated the read.
 */
class GetPremiumStatusUseCase(
    private val gateway: PremiumStatusGateway,
    private val sessions: AuthSessionStore,
) {
    suspend operator fun invoke(): PremiumStatusSnapshot {
        val session = sessions.load() ?: throw PremiumStatusException.SignedOut()
        val status = gateway.status(session.familyId, session.accessToken)
        if (session.accountId.isBlank()) throw PremiumStatusException.SignedOut()
        return PremiumStatusSnapshot(accountId = session.accountId, status = status)
    }
}
