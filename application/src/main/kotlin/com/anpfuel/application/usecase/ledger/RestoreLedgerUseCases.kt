package com.anpfuel.application.usecase.ledger

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.repository.LedgerDraftDecoder
import com.anpfuel.domain.repository.LedgerDraftStore
import com.anpfuel.domain.repository.LedgerErasureLog
import com.anpfuel.domain.repository.LedgerRecordsGateway
import com.anpfuel.domain.repository.LedgerServerRecord
import com.anpfuel.domain.repository.LedgerSyncScope
import kotlinx.coroutines.CancellationException

/** In-memory preview only: process death discards the authority to apply it. */
data class RestorePullItem(val dataset: LedgerDataset, val recordId: String, val serverRevision: Long, val bodyJson: String)

data class RestoreDiff(
    val toPull: List<RestorePullItem>,
    val serverRevisions: Map<String, Long>,
    val localOnlyUnsynced: Int,
    val skippedErased: Int,
    val skippedVoided: Int,
    val truncated: Boolean,
    val accountId: String? = null,
    val sourceOrigin: String = "",
    val createdAtMillis: Long = 0,
)

enum class RestoreItemOutcome { APPLIED, SKIPPED_ERASED, UNDECODABLE, STALE }
data class RestoreItemResult(val dataset: LedgerDataset, val recordId: String, val outcome: RestoreItemOutcome)
sealed interface RestoreOutcome {
    data class Applied(val items: List<RestoreItemResult>) : RestoreOutcome
    data object StaleSnapshot : RestoreOutcome
    data object NothingToApply : RestoreOutcome
}

private const val RESTORE_PAGE_LIMIT = 100
private const val RESTORE_PAGE_CAP = 5
internal const val RESTORE_PREVIEW_TTL_MILLIS = 5 * 60 * 1000L

private suspend fun readDataset(
    records: LedgerRecordsGateway,
    session: com.anpfuel.domain.portable.PortableAuth.Session,
    dataset: LedgerDataset,
): Pair<List<LedgerServerRecord>, Boolean> {
    val rows = mutableListOf<LedgerServerRecord>()
    var cursor: String? = null
    val seen = mutableSetOf<String>()
    repeat(RESTORE_PAGE_CAP) {
        val page = records.list(session.familyId, session.accessToken, dataset.wire, null, RESTORE_PAGE_LIMIT, cursor)
        rows.addAll(page.items)
        cursor = page.nextCursor
        if (cursor == null) return rows to false
        check(seen.add(cursor!!)) { "ledger.repeated-cursor" }
    }
    return rows to true
}

/** Read-only bounded preview. Existing local records are retained; replacing one requires conflict resolution. */
class PreviewLedgerRestoreUseCase(
    private val drafts: LedgerDraftStore,
    private val erasures: LedgerErasureLog,
    private val records: LedgerRecordsGateway,
    private val sessions: AuthSessionStore,
    private val origin: String,
    private val scope: LedgerSyncScope? = null,
    private val commands: com.anpfuel.domain.repository.PremiumCommandStore? = null,
    private val nowMillis: () -> Long = System::currentTimeMillis,
) {
    suspend operator fun invoke(): RestoreDiff {
        val session = sessions.load() ?: throw com.anpfuel.domain.repository.LedgerSyncDenied("ledger.no-proof")
        check(scope?.isDeviceEnabled() != false) { "ledger.device-disabled" }
        val local = drafts.list(session.accountId, origin).associateBy { it.draft.dataset.wire + "|" + it.draft.recordId }
        val pulls = mutableListOf<RestorePullItem>()
        val revisions = mutableMapOf<String, Long>()
        val serverIds = mutableSetOf<String>()
        var erased = 0
        var voided = 0
        var truncated = false
        for (dataset in LedgerDataset.entries) {
            if (scope?.isDatasetEnabled(dataset.wire) == false) continue
            val (rows, cut) = readDataset(records, session, dataset)
            truncated = truncated || cut
            for (row in rows) {
                val key = dataset.wire + "|" + row.id
                check(serverIds.add(key)) { "ledger.duplicate-record" }
                when {
                    row.voided -> voided++
                    erasures.isErased(session.accountId, origin, dataset.wire, row.id) -> erased++
                    commands?.hasHeldRecord(com.anpfuel.domain.premium.PremiumOwnerScope(session.accountId, origin), dataset.wire, row.id) == true -> Unit
                    local[key] == null -> {
                        pulls.add(RestorePullItem(dataset, row.id, row.revision, row.payloadJson))
                        revisions[key] = row.revision
                    }
                }
            }
        }
        check(sessions.load()?.accountId == session.accountId) { "ledger.owner-changed" }
        return RestoreDiff(pulls, revisions, local.size, erased, voided, truncated,
            session.accountId, origin, nowMillis())
    }
}

/** Revalidate before writing. The data binding wraps all local changes in one Room transaction. */
class ConfirmLedgerRestoreUseCase(
    private val drafts: LedgerDraftStore,
    private val erasures: LedgerErasureLog,
    private val records: LedgerRecordsGateway,
    private val sessions: AuthSessionStore,
    private val origin: String,
    private val decoder: LedgerDraftDecoder,
    private val nowMillis: () -> Long = System::currentTimeMillis,
    private val scope: LedgerSyncScope? = null,
    private val commands: com.anpfuel.domain.repository.PremiumCommandStore? = null,
    private val localTransaction: suspend (suspend () -> RestoreOutcome) -> RestoreOutcome = { it() },
) {
    suspend operator fun invoke(diff: RestoreDiff): RestoreOutcome {
        val session = sessions.load() ?: return RestoreOutcome.StaleSnapshot
        if (diff.accountId != session.accountId || diff.sourceOrigin != origin || diff.truncated ||
            nowMillis() < diff.createdAtMillis || nowMillis() - diff.createdAtMillis > RESTORE_PREVIEW_TTL_MILLIS ||
            scope?.isDeviceEnabled() == false || diff.toPull.any { scope?.isDatasetEnabled(it.dataset.wire) == false }) {
            return RestoreOutcome.StaleSnapshot
        }
        if (diff.toPull.isEmpty()) return RestoreOutcome.NothingToApply
        val fresh = mutableMapOf<String, LedgerServerRecord>()
        try {
            for (dataset in diff.toPull.map { it.dataset }.distinct()) {
                val (rows, cut) = readDataset(records, session, dataset)
                if (cut) return RestoreOutcome.StaleSnapshot
                for (row in rows) fresh[dataset.wire + "|" + row.id] = row
            }
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (_: Exception) {
            return RestoreOutcome.StaleSnapshot
        }
        for (item in diff.toPull) {
            val row = fresh[item.dataset.wire + "|" + item.recordId] ?: return RestoreOutcome.StaleSnapshot
            if (row.revision != item.serverRevision || row.voided || row.payloadJson != item.bodyJson) return RestoreOutcome.StaleSnapshot
        }
        if (sessions.load()?.accountId != session.accountId) return RestoreOutcome.StaleSnapshot
        return localTransaction {
            if (sessions.load()?.accountId != session.accountId || scope?.isDeviceEnabled() == false ||
                diff.toPull.any { scope?.isDatasetEnabled(it.dataset.wire) == false }) return@localTransaction RestoreOutcome.StaleSnapshot
            // An edit created after preview is never silently overwritten. Identical replays are harmless.
            for (item in diff.toPull) {
                if (commands?.hasHeldRecord(com.anpfuel.domain.premium.PremiumOwnerScope(session.accountId, origin), item.dataset.wire, item.recordId) == true) {
                    return@localTransaction RestoreOutcome.StaleSnapshot
                }
                val held = drafts.find(session.accountId, origin, item.dataset, item.recordId)
                if (held != null && (held.expectedRevision != item.serverRevision || held.draft.bodyJson() != item.bodyJson)) {
                    return@localTransaction RestoreOutcome.StaleSnapshot
                }
            }
            val results = mutableListOf<RestoreItemResult>()
            for (item in diff.toPull) {
                check(sessions.load()?.accountId == session.accountId && scope?.isDeviceEnabled() != false &&
                    scope?.isDatasetEnabled(item.dataset.wire) != false) { "ledger.participation-changed" }
                if (erasures.isErased(session.accountId, origin, item.dataset.wire, item.recordId)) {
                    results.add(RestoreItemResult(item.dataset, item.recordId, RestoreItemOutcome.SKIPPED_ERASED))
                    continue
                }
                val decoded = try {
                    decoder.decode(item.dataset, item.bodyJson).also {
                        require(it.dataset == item.dataset && it.recordId == item.recordId)
                    }
                } catch (cancelled: CancellationException) {
                    throw cancelled
                } catch (_: Exception) {
                    results.add(RestoreItemResult(item.dataset, item.recordId, RestoreItemOutcome.UNDECODABLE))
                    continue
                }
                if (drafts.find(session.accountId, origin, item.dataset, item.recordId) == null) {
                    drafts.save(session.accountId, origin, decoded, item.serverRevision, nowMillis(), null)
                }
                results.add(RestoreItemResult(item.dataset, item.recordId, RestoreItemOutcome.APPLIED))
            }
            RestoreOutcome.Applied(results)
        }
    }
}
