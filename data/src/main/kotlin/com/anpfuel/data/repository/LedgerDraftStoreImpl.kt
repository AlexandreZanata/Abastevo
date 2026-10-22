package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.LedgerDraftDao
import com.anpfuel.data.local.entity.LedgerDraftEntity
import com.anpfuel.data.mapper.LedgerDraftCodec
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.LedgerDraft
import com.anpfuel.domain.repository.LedgerDraftStore
import com.anpfuel.domain.repository.StoredLedgerDraft
import java.util.UUID
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class LedgerDraftStoreImpl @Inject constructor(
    private val drafts: LedgerDraftDao,
) : LedgerDraftStore {

    override suspend fun save(
        accountId: String,
        origin: String,
        draft: LedgerDraft,
        expectedRevision: Long,
        nowMillis: Long,
    ): StoredLedgerDraft {
        drafts.upsert(
            LedgerDraftEntity(
                draftId = stableDraftId(accountId, origin, draft),
                ownerAccount = accountId,
                origin = origin,
                dataset = draft.dataset.wire,
                recordId = draft.recordId,
                expectedRevision = expectedRevision,
                bodyJson = draft.bodyJson(),
                updatedAtMillis = nowMillis,
            ),
        )
        return StoredLedgerDraft(draft, expectedRevision, nowMillis)
    }

    override suspend fun find(
        accountId: String,
        origin: String,
        dataset: LedgerDataset,
        recordId: String,
    ): StoredLedgerDraft? =
        drafts.findByRecord(accountId, origin, dataset.wire, recordId)?.toDomain()

    override suspend fun list(accountId: String, origin: String): List<StoredLedgerDraft> =
        drafts.listOwned(accountId, origin).map { it.toDomain() }

    override suspend fun delete(accountId: String, origin: String, dataset: LedgerDataset, recordId: String) {
        drafts.deleteByRecord(accountId, origin, dataset.wire, recordId)
    }

    override suspend fun clearOwner(accountId: String, origin: String) {
        drafts.clearOwner(accountId, origin)
    }

    private fun stableDraftId(accountId: String, origin: String, draft: LedgerDraft): String =
        UUID.nameUUIDFromBytes(
            "ledger-draft|$accountId|$origin|${draft.dataset.wire}|${draft.recordId}"
                .toByteArray(Charsets.UTF_8),
        ).toString()

    private fun LedgerDraftEntity.toDomain(): StoredLedgerDraft {
        val dataset = LedgerDataset.parse(dataset)
        return StoredLedgerDraft(
            draft = LedgerDraftCodec.decode(dataset, bodyJson),
            expectedRevision = expectedRevision,
            updatedAtMillis = updatedAtMillis,
        )
    }
}
