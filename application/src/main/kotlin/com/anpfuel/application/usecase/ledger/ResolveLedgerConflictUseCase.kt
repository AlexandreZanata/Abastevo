package com.anpfuel.application.usecase.ledger

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.premium.LedgerCommandFactory
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.PremiumOwnerScope
import com.anpfuel.domain.premium.PremiumValidationException
import com.anpfuel.domain.repository.LedgerDraftDecoder
import com.anpfuel.domain.repository.LedgerDraftStore
import com.anpfuel.domain.repository.LedgerErasureLog
import com.anpfuel.domain.repository.LedgerRecordsGateway
import com.anpfuel.domain.repository.PremiumCommandStore

enum class LedgerConflictChoice { KEEP_MINE, TAKE_SERVER, CANCEL }

sealed interface ResolveOutcome {
    data class AppliedKeepMine(val newRevision: Long) : ResolveOutcome
    data object AppliedTakeServer : ResolveOutcome
    data object Cancelled : ResolveOutcome
    data object NoConflict : ResolveOutcome
    data object NoContent : ResolveOutcome
    data class Undecodable(val reason: String) : ResolveOutcome
}

/**
 * MEX-15 explicit conflict resolution. Never auto-merges, never drops
 * held work without an explicit choice:
 * - KEEP_MINE rebases the held draft onto the server revision and
 *   re-queues one command without scheduling an upload.
 * - TAKE_SERVER adopts the server payload, clears local queued work
 *   for the record and records no erasure (the server still holds it).
 * - Server-missing TAKE_SERVER drops the local draft and records the
 *   erasure so a later old restore cannot resurrect it.
 * - CANCEL keeps everything exactly as held.
 */
class ResolveLedgerConflictUseCase(
    private val drafts: LedgerDraftStore,
    private val commands: PremiumCommandStore,
    private val records: LedgerRecordsGateway,
    private val sessions: AuthSessionStore,
    private val origin: String,
    private val erasures: LedgerErasureLog,
    private val decoder: LedgerDraftDecoder,
    private val nowMillis: () -> Long = System::currentTimeMillis,
    private val localTransaction: suspend (suspend () -> ResolveOutcome) -> ResolveOutcome = { it() },
) {

    suspend operator fun invoke(
        dataset: LedgerDataset,
        recordId: String,
        choice: LedgerConflictChoice,
    ): ResolveOutcome {
        if (choice == LedgerConflictChoice.CANCEL) return ResolveOutcome.Cancelled
        val session = sessions.load() ?: throw com.anpfuel.domain.repository.LedgerSyncDenied("ledger.no-proof")
        val accountId = session.accountId
        val scope = PremiumOwnerScope(accountId, origin)
        val stored = drafts.find(accountId, origin, dataset, recordId)
        val heldDelete = if (stored == null) commands.listConflicts(scope, 200)
            .firstOrNull { it.dataset == dataset.wire && it.recordId == recordId && it.delete } else null
        if (stored == null && heldDelete == null) return ResolveOutcome.NoConflict
        val server = fetchServer(session, dataset, stored?.draft?.let(::vehicleOf), recordId)
        check(sessions.load()?.accountId == accountId) { "ledger.owner-changed" }
        return localTransaction {
            check(sessions.load()?.accountId == accountId) { "ledger.owner-changed" }
            check(drafts.find(accountId, origin, dataset, recordId) == stored) { "ledger.local-changed" }
            when (choice) {
                LedgerConflictChoice.CANCEL -> ResolveOutcome.Cancelled
                LedgerConflictChoice.KEEP_MINE -> {
                    // Missing/deleted content cannot be recreated under a tombstoned source identity.
                    if (server == null || server.voided) return@localTransaction ResolveOutcome.NoContent
                    val rebased = if (stored != null) LedgerCommandFactory.toCommand(scope, stored.draft, server.revision)
                    else heldDelete!!.copy(expectedRevision = server.revision, operationId = java.util.UUID.nameUUIDFromBytes(
                        "resolve-delete|${heldDelete.operationId}|${server.revision}".toByteArray(Charsets.UTF_8)).toString())
                    dropQueuedForRecord(scope, dataset, recordId)
                    if (stored != null) drafts.save(accountId, origin, stored.draft, server.revision, nowMillis(), stored.context)
                    commands.enqueue(rebased)
                    ResolveOutcome.AppliedKeepMine(server.revision)
                }
                LedgerConflictChoice.TAKE_SERVER -> {
                    if (server == null || server.voided) {
                        dropQueuedForRecord(scope, dataset, recordId)
                        erasures.recordErased(accountId, origin, dataset.wire, recordId, nowMillis())
                        drafts.delete(accountId, origin, dataset, recordId)
                        return@localTransaction ResolveOutcome.AppliedTakeServer
                    }
                    if (erasures.isErased(accountId, origin, dataset.wire, recordId)) return@localTransaction ResolveOutcome.NoContent
                    val decoded = try {
                        decoder.decode(dataset, server.payloadJson)
                    } catch (e: PremiumValidationException) {
                        return@localTransaction ResolveOutcome.Undecodable(e.code)
                    } catch (_: Exception) {
                        return@localTransaction ResolveOutcome.Undecodable("corrupt-payload")
                    }
                    if (decoded.dataset != dataset || decoded.recordId != recordId) return@localTransaction ResolveOutcome.Undecodable("identity-mismatch")
                    dropQueuedForRecord(scope, dataset, recordId)
                    drafts.save(accountId, origin, decoded, server.revision, nowMillis(), stored?.context)
                    ResolveOutcome.AppliedTakeServer
                }
            }
        }
    }

    private suspend fun fetchServer(
        session: com.anpfuel.domain.portable.PortableAuth.Session,
        dataset: LedgerDataset,
        vehicleId: String?,
        recordId: String,
    ): com.anpfuel.domain.repository.LedgerServerRecord? {
        var cursor: String? = null
        repeat(5) {
            val page = records.list(session.familyId, session.accessToken, dataset.wire, vehicleId, 100, cursor)
            page.items.firstOrNull { it.id == recordId }?.let { return it }
            cursor = page.nextCursor
            if (cursor == null) return null
        }
        throw java.io.IOException("ledger.truncated")
    }

    private suspend fun dropQueuedForRecord(scope: PremiumOwnerScope, dataset: LedgerDataset, recordId: String) {
        commands.removeRecord(scope, dataset.wire, recordId)
    }

    private fun vehicleOf(draft: com.anpfuel.domain.premium.LedgerDraft): String? = when (draft) {
        is com.anpfuel.domain.premium.RefuelDraft -> draft.vehicleId
        is com.anpfuel.domain.premium.ExpenseDraft -> draft.vehicleId
        is com.anpfuel.domain.premium.ChargeDraft -> draft.vehicleId
    }
}
