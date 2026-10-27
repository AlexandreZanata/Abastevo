package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.LedgerDraftDao
import com.anpfuel.data.local.entity.LedgerDraftEntity
import com.anpfuel.data.mapper.LedgerDraftCodec
import com.anpfuel.data.mapper.LedgerOdometerEnvelope
import com.anpfuel.domain.premium.*
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.LedgerDraft
import com.anpfuel.domain.premium.LedgerSessionContext
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
        context: LedgerSessionContext?,
        odometerEvidence: OdometerEvidence,
    ): StoredLedgerDraft {
        val previous = drafts.findByRecord(accountId, origin, draft.dataset.wire, draft.recordId)
        val receipt = previous?.let { LedgerOdometerEnvelope.read(it.bodyJson).receipt }
        drafts.upsert(
            LedgerDraftEntity(
                draftId = stableDraftId(accountId, origin, draft),
                ownerAccount = accountId,
                origin = origin,
                dataset = draft.dataset.wire,
                recordId = draft.recordId,
                expectedRevision = expectedRevision,
                bodyJson = LedgerOdometerEnvelope.write(draft, odometerEvidence, receipt),
                updatedAtMillis = nowMillis,
                context = context?.wire,
            ),
        )
        return StoredLedgerDraft(draft, expectedRevision, nowMillis, context, odometerEvidence)
    }

    override suspend fun reviewOdometer(
        accountId: String, origin: String, dataset: LedgerDataset, recordId: String,
        expectedBodySha256: String, choice: OdometerReviewChoice, nowMillis: Long,
    ): StoredLedgerDraft {
        val row = drafts.findByRecord(accountId, origin, dataset.wire, recordId)
            ?: throw PremiumValidationException("draft-missing")
        fun replay(entity: LedgerDraftEntity): StoredLedgerDraft? {
            val stored = entity.toDomain()
            val receipt = LedgerOdometerEnvelope.read(entity.bodyJson).receipt ?: return null
            return stored.takeIf {
                receipt.base == expectedBodySha256 && receipt.choice == choice.name &&
                    receipt.revision == entity.expectedRevision &&
                    receipt.result == LegacyOdometerReview.fingerprint(stored.draft) &&
                    stored.odometerEvidence == OdometerEvidence.VERIFIED_METRES
            }
        }
        replay(row)?.let { return it }
        val stored = row.toDomain()
        if (stored.odometerEvidence != OdometerEvidence.UNVERIFIED ||
            LegacyOdometerReview.fingerprint(stored.draft) != expectedBodySha256
        ) throw PremiumValidationException("stale-odometer-review")
        val preview = LegacyOdometerReview.preview(stored.draft, choice)
        val corrected = LegacyOdometerReview.corrected(stored.draft, preview.correctedMetres)
        val receipt = LedgerOdometerEnvelope.Receipt(preview.originalValue, expectedBodySha256,
            row.expectedRevision, choice.name, LegacyOdometerReview.fingerprint(corrected))
        val body = LedgerOdometerEnvelope.write(corrected, OdometerEvidence.VERIFIED_METRES, receipt)
        val count = drafts.compareAndSetBody(accountId,origin,dataset.wire,recordId,row.expectedRevision,row.bodyJson,body,nowMillis,row.draftId)
        if (count != 1) {
            val current = drafts.findByRecord(accountId,origin,dataset.wire,recordId)
            current?.let { replay(it) }?.let { return it }
            throw PremiumValidationException("stale-odometer-review")
        }
        return row.copy(bodyJson = body, updatedAtMillis = nowMillis).toDomain()
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
            context = context?.let { LedgerSessionContext.parse(it) },
            odometerEvidence = LedgerOdometerEnvelope.read(bodyJson).evidence,
        )
    }
}
