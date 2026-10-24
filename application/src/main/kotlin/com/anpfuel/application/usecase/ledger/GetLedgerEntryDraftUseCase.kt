package com.anpfuel.application.usecase.ledger

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.repository.LedgerDraftStore
import com.anpfuel.domain.repository.StoredLedgerDraft

/**
 * MEX-12 draft fetch for edit prefill: the stored validated draft plus
 * its expected revision, so corrections keep the revision chain instead
 * of forking a duplicate entry.
 */
class GetLedgerEntryDraftUseCase(
    private val drafts: LedgerDraftStore,
    private val sessions: AuthSessionStore,
    private val origin: String,
) {

    suspend operator fun invoke(dataset: LedgerDataset, recordId: String): StoredLedgerDraft? {
        val accountId = sessions.load()?.accountId ?: "local"
        return drafts.find(accountId, origin, dataset, recordId)
    }
}
