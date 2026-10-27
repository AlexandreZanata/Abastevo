package com.anpfuel.application.usecase.ledger

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.premium.LedgerCommandFactory
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.LedgerDraft
import com.anpfuel.domain.premium.PremiumCommand
import com.anpfuel.domain.premium.PremiumOwnerScope
import com.anpfuel.domain.premium.PremiumValidationException
import com.anpfuel.domain.repository.LedgerDraftStore
import com.anpfuel.domain.repository.PremiumCommandStore
import com.anpfuel.domain.repository.StoredLedgerDraft

/**
 * MEX-11 durable ledger drafts and idempotent command enqueue.
 *
 * Drafts validate at construction (see [LedgerDraft]); saving replaces
 * the same record so double-save converges. Enqueue derives a stable
 * operation ID from owner, dataset, record, revision and body, so a lost
 * acknowledgement retries with the same ID instead of duplicating.
 * Recovery releases crashed claims process-wide (leases are time, not
 * owner, bound). No transport happens here (MEX-14 owns dispatch).
 */
class SaveLedgerDraftUseCase(
    private val drafts: LedgerDraftStore,
    private val sessions: AuthSessionStore,
    private val origin: String,
    private val nowMillis: () -> Long = System::currentTimeMillis,
) {

    suspend operator fun invoke(
        draft: LedgerDraft,
        expectedRevision: Long,
        context: com.anpfuel.domain.premium.LedgerSessionContext? = null,
        odometerEvidence: com.anpfuel.domain.premium.OdometerEvidence = com.anpfuel.domain.premium.OdometerEvidence.UNVERIFIED,
    ): StoredLedgerDraft {
        if (expectedRevision < 0) throw PremiumValidationException("invalid-revision")
        val accountId = sessions.load()?.accountId ?: "local"
        return drafts.save(accountId, origin, draft, expectedRevision, nowMillis(), context, odometerEvidence)
    }
    suspend fun reviewOdometer(
        dataset: LedgerDataset, recordId: String, expectedBodySha256: String,
        choice: com.anpfuel.domain.premium.OdometerReviewChoice,
    ): StoredLedgerDraft {
        if (!Regex("[0-9a-f]{64}").matches(expectedBodySha256)) throw PremiumValidationException("invalid-review-base")
        val accountId = sessions.load()?.accountId ?: "local"
        val reviewed = drafts.reviewOdometer(accountId, origin, dataset, recordId, expectedBodySha256, choice, nowMillis())
        if ((sessions.load()?.accountId ?: "local") != accountId) throw PremiumValidationException("owner-scope-changed")
        return reviewed
    }

}

class EnqueueLedgerCommandUseCase(
    private val drafts: LedgerDraftStore,
    private val commands: PremiumCommandStore,
    private val sessions: AuthSessionStore,
    private val origin: String,
) {

    suspend operator fun invoke(
        dataset: LedgerDataset,
        recordId: String,
    ): PremiumCommand {
        val accountId = sessions.load()?.accountId ?: "local"
        val stored = drafts.find(accountId, origin, dataset, recordId)
            ?: throw PremiumValidationException("draft-missing")
        val scope = PremiumOwnerScope(accountId, origin)
        val command = LedgerCommandFactory.toCommand(scope, stored.draft, stored.expectedRevision)
        commands.enqueue(command)
        return command
    }
}

class RecoverLedgerOutboxUseCase(
    private val commands: PremiumCommandStore,
    private val nowMillis: () -> Long = System::currentTimeMillis,
) {

    suspend operator fun invoke(): Int = commands.releaseExpired(nowMillis())
}
