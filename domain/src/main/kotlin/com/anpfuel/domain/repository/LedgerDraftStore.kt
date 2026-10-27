package com.anpfuel.domain.repository

import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.LedgerDraft
import com.anpfuel.domain.premium.LedgerSessionContext

/** Stored draft envelope: validated values plus dispatch revision. */
data class StoredLedgerDraft(
    val draft: LedgerDraft,
    val expectedRevision: Long,
    val updatedAtMillis: Long,
    val context: LedgerSessionContext? = null,
    val odometerEvidence: com.anpfuel.domain.premium.OdometerEvidence = com.anpfuel.domain.premium.OdometerEvidence.UNVERIFIED,
)

/**
 * MEX-11 durable ledger drafts. Partition keyed like every owner store;
 * saving the same record replaces the row.
 */
interface LedgerDraftStore {

    suspend fun save(
        accountId: String,
        origin: String,
        draft: LedgerDraft,
        expectedRevision: Long,
        nowMillis: Long,
        context: LedgerSessionContext? = null,
        odometerEvidence: com.anpfuel.domain.premium.OdometerEvidence = com.anpfuel.domain.premium.OdometerEvidence.UNVERIFIED,
    ): StoredLedgerDraft

    suspend fun reviewOdometer(
        accountId: String, origin: String, dataset: LedgerDataset, recordId: String,
        expectedBodySha256: String, choice: com.anpfuel.domain.premium.OdometerReviewChoice,
        nowMillis: Long,
    ): StoredLedgerDraft = throw com.anpfuel.domain.premium.PremiumValidationException("odometer-review-unavailable")

    suspend fun find(
        accountId: String,
        origin: String,
        dataset: LedgerDataset,
        recordId: String,
    ): StoredLedgerDraft?

    suspend fun list(accountId: String, origin: String): List<StoredLedgerDraft>

    suspend fun delete(accountId: String, origin: String, dataset: LedgerDataset, recordId: String)

    suspend fun clearOwner(accountId: String, origin: String)
}
