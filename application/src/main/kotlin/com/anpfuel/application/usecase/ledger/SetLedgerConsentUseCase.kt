package com.anpfuel.application.usecase.ledger

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.repository.LedgerConsentGateway
import com.anpfuel.domain.repository.LedgerConsentStatus
import com.anpfuel.domain.repository.LedgerSyncDenied

/**
 * Records or withdraws the account's server consent for ledger uploads.
 *
 * The backend refuses every upsert of a dataset outside the consent mask, so
 * an opt-in counts only once the answer contains every requested dataset, and
 * a withdrawal only once none of them remains. Each dataset is sent once, in
 * canonical order, with the stored session of the signed-in owner.
 */
class SetLedgerConsentUseCase(
    private val gateway: LedgerConsentGateway,
    private val sessions: AuthSessionStore,
) {
    suspend operator fun invoke(datasets: Collection<LedgerDataset>, enabled: Boolean): LedgerConsentStatus {
        val requested = LedgerDataset.entries.filter { it in datasets }
        require(requested.isNotEmpty()) { "ledger.no-dataset" }
        val session = sessions.load() ?: throw LedgerSyncDenied("ledger.no-proof")
        val status = gateway.setConsent(session.accountId, session.familyId, session.accessToken, requested, enabled)
        val applied = if (enabled) status.datasets.containsAll(requested) else requested.none { it in status.datasets }
        if (!applied) throw LedgerSyncDenied("ledger.consent-not-applied")
        return status
    }
}
