package com.anpfuel.application.usecase.ledger

import com.anpfuel.application.port.EstimatedEfficiencyStore
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.premium.ChargeDraft
import com.anpfuel.domain.premium.EstimatedEfficiency
import com.anpfuel.domain.premium.ExpenseCategory
import com.anpfuel.domain.premium.ExpenseDraft
import com.anpfuel.domain.premium.LedgerCommandFactory
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.LedgerDraft
import com.anpfuel.domain.premium.LedgerFill
import com.anpfuel.domain.premium.LedgerGapIntent
import com.anpfuel.domain.premium.LedgerIntentCommandFactory
import com.anpfuel.domain.premium.LedgerSegmentReset
import com.anpfuel.domain.premium.LedgerSessionContext
import com.anpfuel.domain.premium.PremiumCommand
import com.anpfuel.domain.premium.PremiumOwnerScope
import com.anpfuel.domain.premium.PremiumValidationException
import com.anpfuel.domain.premium.RefuelDraft
import com.anpfuel.domain.repository.LedgerDraftStore
import com.anpfuel.domain.repository.PremiumCommandStore
import com.anpfuel.domain.repository.VehicleRepository
import com.anpfuel.domain.valueobject.DomainId
import java.time.Instant

/** Truthful entry state: draft only, queued, or diverged from the queue. */
enum class LedgerEntryState { LOCAL, PENDING, CONFLICT }

sealed interface LedgerEntrySummary {
    data class Refuel(val quantityMilli: Long, val totalCentavos: Long, val fill: LedgerFill) : LedgerEntrySummary
    data class Expense(val amountCentavos: Long, val category: ExpenseCategory) : LedgerEntrySummary
    data class Charge(val energyWh: Long?, val totalCentavos: Long?) : LedgerEntrySummary
}

data class LedgerEntryRow(
    val recordId: String,
    val dataset: LedgerDataset,
    val vehicleId: String,
    val vehicleName: String,
    val occurredAt: Instant,
    val timezone: String,
    val summary: LedgerEntrySummary,
    val expectedRevision: Long,
    val note: String?,
    val state: LedgerEntryState,
    val context: LedgerSessionContext?,
)

private fun summaryOf(draft: LedgerDraft): LedgerEntrySummary = when (draft) {
    is RefuelDraft -> LedgerEntrySummary.Refuel(draft.quantityMilli, draft.totalCentavos, draft.fill)
    is ExpenseDraft -> LedgerEntrySummary.Expense(draft.amountCentavos, draft.category)
    is ChargeDraft -> LedgerEntrySummary.Charge(draft.energyWh, draft.totalCentavos)
}

/**
 * MEX-12 entry list: local drafts joined with queued commands for one
 * vehicle. A queued command with the same revision reads PENDING; a
 * different revision reads CONFLICT (correction drafted after enqueue);
 * no queue reads LOCAL. Queued-only tombstones are not entries.
 */
class ListLedgerEntriesUseCase(
    private val drafts: LedgerDraftStore,
    private val commands: PremiumCommandStore,
    private val sessions: AuthSessionStore,
    private val origin: String,
    private val vehicles: VehicleRepository,
) {

    suspend operator fun invoke(vehicleId: DomainId): List<LedgerEntryRow> {
        val accountId = sessions.load()?.accountId ?: "local"
        val queued = commands.listPending(PremiumOwnerScope(accountId, origin), 50)
            .filter { it.dataset == LedgerDataset.REFUEL.wire || it.dataset == LedgerDataset.EXPENSE.wire || it.dataset == LedgerDataset.CHARGE.wire }
            .associateBy { it.dataset to it.recordId }
        return drafts.list(accountId, origin)
            .filter { rowVehicle(it.draft) == vehicleId.value }
            .map { stored ->
                val draft = stored.draft
                val queue = queued[draft.dataset.wire to draft.recordId]
                val state = when {
                    queue == null -> LedgerEntryState.LOCAL
                    queue.expectedRevision == stored.expectedRevision -> LedgerEntryState.PENDING
                    else -> LedgerEntryState.CONFLICT
                }
                LedgerEntryRow(
                    recordId = draft.recordId,
                    dataset = draft.dataset,
                    vehicleId = rowVehicle(draft),
                    vehicleName = vehicles.findById(DomainId.from(rowVehicle(draft)))?.displayName.orEmpty(),
                    occurredAt = rowOccurred(draft),
                    timezone = rowTimezone(draft),
                    summary = summaryOf(draft),
                    expectedRevision = stored.expectedRevision,
                    note = rowNote(draft),
                    state = state,
                    context = stored.context,
                )
            }.sortedBy { it.occurredAt }
    }

    private fun rowVehicle(draft: LedgerDraft): String = when (draft) {
        is RefuelDraft -> draft.vehicleId
        is ExpenseDraft -> draft.vehicleId
        is ChargeDraft -> draft.vehicleId
    }

    private fun rowOccurred(draft: LedgerDraft): Instant = when (draft) {
        is RefuelDraft -> draft.occurredAt
        is ExpenseDraft -> draft.occurredAt
        is ChargeDraft -> draft.occurredAt
    }

    private fun rowTimezone(draft: LedgerDraft): String = when (draft) {
        is RefuelDraft -> draft.timezone
        is ExpenseDraft -> draft.timezone
        is ChargeDraft -> draft.timezone
    }

    private fun rowNote(draft: LedgerDraft): String? = when (draft) {
        is RefuelDraft -> draft.note
        is ExpenseDraft -> draft.note
        is ChargeDraft -> draft.note
    }
}

/**
 * MEX-12 entry delete: drops the local draft and queues a stable
 * tombstone once (convergent on retry). Deleting a record that was
 * never synced only clears local state; the tombstone is a no-op
 * server-side no-op guard, never a data loss vector.
 */
class DeleteLedgerEntryUseCase(
    private val drafts: LedgerDraftStore,
    private val commands: PremiumCommandStore,
    private val sessions: AuthSessionStore,
    private val origin: String,
    private val erasures: com.anpfuel.domain.repository.LedgerErasureLog? = null,
    private val localTransaction: suspend (suspend () -> PremiumCommand) -> PremiumCommand = { it() },
) {

    suspend operator fun invoke(dataset: LedgerDataset, recordId: String): PremiumCommand {
        val accountId = sessions.load()?.accountId ?: "local"
        return localTransaction {
            erasures?.recordErased(accountId, origin, dataset.wire, recordId, System.currentTimeMillis())
            drafts.delete(accountId, origin, dataset, recordId)
            val tombstone = LedgerCommandFactory.deleteCommand(PremiumOwnerScope(accountId, origin), dataset, recordId)
            commands.enqueue(tombstone)
            tombstone
        }
    }
}

class DeclareLedgerGapUseCase(
    private val commands: PremiumCommandStore,
    private val sessions: AuthSessionStore,
    private val origin: String,
) {

    suspend operator fun invoke(vehicleId: DomainId, intent: LedgerGapIntent): PremiumCommand {
        val accountId = sessions.load()?.accountId ?: "local"
        val command = LedgerIntentCommandFactory.gapCommand(
            PremiumOwnerScope(accountId, origin),
            vehicleId.value,
            intent,
        )
        commands.enqueue(command)
        return command
    }
}

class ResetLedgerSegmentUseCase(
    private val commands: PremiumCommandStore,
    private val sessions: AuthSessionStore,
    private val origin: String,
) {

    suspend operator fun invoke(vehicleId: DomainId, reset: LedgerSegmentReset): PremiumCommand {
        val accountId = sessions.load()?.accountId ?: "local"
        val command = LedgerIntentCommandFactory.resetCommand(
            PremiumOwnerScope(accountId, origin),
            vehicleId.value,
            reset,
        )
        commands.enqueue(command)
        return command
    }
}

/**
 * MEX-12 estimated efficiency configuration: validated km/L stored
 * locally per vehicle, cleared on demand. Never uploaded, never a
 * measurement claim.
 */
class EstimatedEfficiencyUseCase(
    private val store: EstimatedEfficiencyStore,
) {

    fun current(vehicle: Vehicle): String? =
        store.get(vehicle.id.value)?.let { raw ->
            runCatching { EstimatedEfficiency.of(raw).toPlainString() }.getOrNull()
        }

    fun set(vehicle: Vehicle, input: String): String {
        val parsed = EstimatedEfficiency.of(input)
        store.set(vehicle.id.value, parsed.toPlainString())
        return parsed.toPlainString()
    }

    fun clear(vehicle: Vehicle) {
        store.clear(vehicle.id.value)
    }
}
