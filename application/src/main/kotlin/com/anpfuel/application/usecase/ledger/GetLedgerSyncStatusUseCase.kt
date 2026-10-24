package com.anpfuel.application.usecase.ledger

import com.anpfuel.application.port.LedgerSyncFlagProvider
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.PremiumOwnerScope
import com.anpfuel.domain.repository.LedgerSyncAckStore
import com.anpfuel.domain.repository.LedgerSyncScope
import com.anpfuel.domain.repository.PremiumCommandStore

data class DatasetSyncStatus(
    val dataset: LedgerDataset,
    val pending: Int,
    val conflicts: Int,
    val lastAckRevision: Long?,
    val lastAckAtMillis: Long?,
    val enabled: Boolean,
)

data class LedgerConflictRecord(val dataset: LedgerDataset, val recordId: String)

data class LedgerSyncStatus(
    val accountId: String?,
    val masterEnabled: Boolean,
    val deviceEnabled: Boolean,
    val datasets: List<DatasetSyncStatus>,
    val conflictRecords: List<LedgerConflictRecord> = emptyList(),
)

/**
 * MEX-15 honest sync status: pending counts (bounded), conflicts held
 * for explicit resolution, and last server ack per dataset. Pending
 * local edits are never reported as backed up.
 */
class GetLedgerSyncStatusUseCase(
    private val commands: PremiumCommandStore,
    private val acks: LedgerSyncAckStore,
    private val sessions: AuthSessionStore,
    private val origin: String,
    private val flags: LedgerSyncFlagProvider,
    private val scope: LedgerSyncScope,
) {

    suspend fun invoke(): LedgerSyncStatus {
        val accountId = sessions.load()?.accountId
        if (accountId == null) {
            return LedgerSyncStatus(null, flags.isEnabled(), scope.isDeviceEnabled(), emptyList())
        }
        val ownerScope = PremiumOwnerScope(accountId, origin)
        val conflicts = commands.listConflicts(ownerScope, 50)
        val byDataset = conflicts.groupBy { it.dataset to it.recordId }.keys
        val datasets = LedgerDataset.entries.map { dataset ->
            val pending = commands.countPending(ownerScope, Int.MAX_VALUE, dataset.wire)
            val ack = acks.lastAck(accountId, origin, dataset.wire)
            DatasetSyncStatus(
                dataset = dataset,
                pending = pending,
                conflicts = byDataset.count { it.first == dataset.wire },
                lastAckRevision = ack?.revision,
                lastAckAtMillis = ack?.atMillis,
                enabled = scope.isDatasetEnabled(dataset.wire),
            )
        }
        check(sessions.load()?.accountId == accountId) { "ledger.owner-changed" }
        return LedgerSyncStatus(accountId, flags.isEnabled(), scope.isDeviceEnabled(), datasets,
            byDataset.mapNotNull { (wire, id) -> LedgerDataset.entries.find { it.wire == wire }?.let { LedgerConflictRecord(it, id) } })
    }
}
