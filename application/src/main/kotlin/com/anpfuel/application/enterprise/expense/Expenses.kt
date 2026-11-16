package com.anpfuel.application.enterprise.expense

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.attachment.EnterpriseReceipts
import com.anpfuel.application.enterprise.attachment.ReceiptDraft
import com.anpfuel.application.enterprise.attachment.ReceiptSaveRefused
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.queue.EnterpriseQueueProducer
import com.anpfuel.application.enterprise.queue.EnterpriseQueueProducerDescriptor
import com.anpfuel.application.enterprise.queue.EnterpriseQueueSource
import com.anpfuel.application.enterprise.queue.EnterpriseQueueTransaction
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.attachment.AttachmentLocalFailure
import com.anpfuel.domain.enterprise.attachment.AttachmentTarget
import com.anpfuel.domain.enterprise.attachment.AttachmentView
import com.anpfuel.domain.enterprise.document.DocumentTargetKind
import com.anpfuel.domain.enterprise.expense.ExpenseEnvelope
import com.anpfuel.domain.enterprise.expense.ExpenseOperationException
import com.anpfuel.domain.enterprise.expense.ExpenseOperations
import com.anpfuel.domain.enterprise.expense.ExpensePresentation
import com.anpfuel.domain.enterprise.expense.ExpenseView
import com.anpfuel.domain.enterprise.fleet.EnterpriseAssetChoice
import com.anpfuel.domain.enterprise.fueling.FuelingEntry
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.trip.TripDeclarations
import java.time.ZoneId
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/*
 * EMP-EXP-004 — the offline direct-expense producer (B-BR-EMP-517..521, EDR-336..338). The operation is the whole
 * record; acceptance is only ever read from its own retained receipt; a receipt photo is a separate EMP-DOC-004
 * capture linked with target `EXPENSE`.
 */

/** The partition's retained `expense.submit` entries, read in one protected snapshot (EDR-336). */
fun interface EnterpriseExpenseStore {
    suspend fun read(attempt: EnterpriseQueueAttempt): List<EnterpriseQueueEntry>
}

/** The registered `expense.submit` producer: it admits only an operation the device decoder reads back (B-BR-EMP-517). */
class EnterpriseExpenseProducer : EnterpriseQueueProducer {
    override fun descriptor(): EnterpriseQueueProducerDescriptor = EnterpriseQueueProducerDescriptor(ExpenseOperations.SUBMIT)
    override fun save(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation) {
        require(operation.type == ExpenseOperations.SUBMIT)
        // Inside the protected write: an expense that the intake parser would not read is never queued.
        ExpenseOperations.read(operation)
    }
    /** The receipt is the truth; no local index changes (EDR-336). */
    override fun acknowledge(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
        ack: OperationAcknowledgement) = Unit
    override fun toString(): String = "EnterpriseExpenseProducer(redacted)"
}

/** The payer of a direct expense (EMP-EXP-002 `payer`). */
enum class ExpensePayer { COMPANY, EMPLOYEE }

/** What the member entered, as typed (EDR-337); the service converts it exactly or refuses it. */
class ExpenseForm(val category: String, val paidTotal: String, val companyAmount: String, val payer: ExpensePayer) {
    override fun toString(): String = "ExpenseForm(redacted)"
}

/** The entered values do not fit their profile; nothing was saved. */
class ExpenseCaptureInvalid : IllegalArgumentException("Expense capture invalid")

/** One saved expense: its operation identifier and the expense (record) identifier. */
class ExpenseSaved(val operationId: String, val expenseId: String) {
    override fun toString(): String = "ExpenseSaved(redacted)"
}

/** Offline expense actions of one partition; every write is durable before it returns (BR-OFFL-002). */
class EnterpriseExpenseService(
    private val source: EnterpriseQueueSource,
    private val save: suspend (EnterprisePartition, EnterpriseOperation) -> Boolean,
    private val store: EnterpriseExpenseStore,
    private val zone: () -> ZoneId,
    private val wallMillis: () -> Long = System::currentTimeMillis,
    private val newId: () -> String = { java.util.UUID.randomUUID().toString() },
) {
    private suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt {
        account.assertCurrent()
        if (!account.identity.owns(partition)) throw EnterpriseLocalDenied()
        val attempt = source.attempt(account, partition)
        if (attempt.account !== account || attempt.partition != partition) throw EnterpriseLocalDenied()
        attempt.assertCurrent()
        return attempt
    }

    suspend fun expenses(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): List<ExpenseView> {
        val attempt = attempt(account, partition)
        val entries = store.read(attempt)
        attempt.assertCurrent()
        return ExpensePresentation.views(entries)
    }

    /** Builds and durably queues one `expense.submit` in the context's own partition (B-BR-EMP-518). */
    suspend fun record(account: EnterpriseQueueAccountLease, context: EnterpriseContextSummary, choice: EnterpriseAssetChoice,
        form: ExpenseForm): ExpenseSaved {
        val partition = context.partition
        if (choice.organizationId != partition.organizationId) throw EnterpriseLocalDenied()
        val attempt = attempt(account, partition)
        if (attempt.enrollment.target.membershipId != context.membershipId) throw EnterpriseLocalDenied()
        // Exact reais → centavos (B-BR-EMP-519); the builder then refuses a company share above the paid total.
        val paid = FuelingEntry.money(form.paidTotal) ?: throw ExpenseCaptureInvalid()
        val company = FuelingEntry.money(form.companyAmount) ?: throw ExpenseCaptureInvalid()
        if (form.category !in ExpenseOperations.DIRECT_CATEGORIES) throw ExpenseCaptureInvalid()
        val operationId = newId()
        val expenseId = newId()
        val declared = TripDeclarations.at(wallMillis(), zone())
        val envelope = ExpenseEnvelope(SyncBinding.of(partition.ownerAccount, partition.origin, partition.organizationId),
            attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId, operationId, declared)
        val operation = try { ExpenseOperations.submit(envelope, expenseId, choice.assetId, form.category, paid, company, form.payer.name, null) }
            catch (_: ExpenseOperationException) { throw ExpenseCaptureInvalid() }
        save(partition, operation)
        return ExpenseSaved(operationId, expenseId)
    }
    override fun toString(): String = "EnterpriseExpenseService(redacted)"
}

data class ExpenseViewState(
    val generation: Long = 0,
    val epoch: Long? = null,
    val context: EnterpriseContextSummary? = null,
    val busy: Boolean = false,
    /** The protected queue was read; an empty list then means no expense on this device. */
    val loaded: Boolean = false,
    val expenses: List<ExpenseView> = emptyList(),
    val openExpenseId: String? = null,
    val captureInvalid: Boolean = false,
    /** The latest write failed; nothing was saved (BR-OFFL-002). */
    val saveFailed: Boolean = false,
    /** The operation identifier of the latest durably saved expense, for one polite announcement. */
    val lastSaved: String? = null,
    val sendFailed: Boolean = false,
    val error: EnterpriseAccessError? = null,
    val unlockRequired: Boolean = false,
    val unrecoverable: Boolean = false,
    /** EMP-DOC-004 receipts by owning operation, each with its own label (B-BR-EMP-454). */
    val attachments: Map<String, List<AttachmentView>> = emptyMap(),
    val attachmentsUnavailable: Boolean = false,
    /** The latest receipt was refused on the device with this reason; nothing of it was saved. */
    val attachmentRefusal: AttachmentLocalFailure? = null,
    /** The expense was saved but its receipt was not (a failed second write); the photo stays in memory for another attempt. */
    val receiptNotSaved: Boolean = false,
    /** The owning operation of the latest durably saved receipt. */
    val lastAttached: String? = null,
) {
    val openExpense: ExpenseView? get() = openExpenseId?.let { id -> expenses.firstOrNull { it.fact.expenseId == id } }
    override fun toString(): String = "ExpenseViewState(redacted)"
}

/** One visible expense owner per screen, bound to account, epoch and background lease like the fueling owner. */
class ExpenseFlow(
    private val source: EnterpriseQueueSource,
    private val service: EnterpriseExpenseService,
    private val fence: EnterpriseSessionFence,
    private val receipts: EnterpriseReceipts? = null,
    private val deliver: suspend (EnterpriseQueueAccountLease, EnterprisePartition) -> Unit,
) {
    private val mutable = MutableStateFlow(ExpenseViewState())
    val state: StateFlow<ExpenseViewState> = mutable.asStateFlow()
    private val monitor = Any()
    private var serial = 0L
    private var closed = false
    private class Owner(val token: Long, val context: EnterpriseContextSummary, val epoch: Long, val background: Long,
        var account: EnterpriseQueueAccountLease? = null)
    private var owner: Owner? = null

    private fun next(): Long = Math.addExact(serial, 1).also { serial = it }
    private fun viewCurrent(request: Owner): Boolean = synchronized(monitor) {
        !closed && owner === request && mutable.value.generation == request.token &&
            fence.epoch == request.epoch && fence.backgroundEpoch == request.background
    }
    private fun retained(request: Owner): Boolean {
        if (!viewCurrent(request)) return false
        if (runCatching { request.account?.assertCurrent() }.isFailure) return false
        return viewCurrent(request)
    }
    private fun clearLocked(error: EnterpriseAccessError? = null) {
        owner = null
        mutable.value = ExpenseViewState(generation = next(), error = error)
    }
    fun withdraw() = synchronized(monitor) { clearLocked() }
    fun close() = synchronized(monitor) { if (!closed) { clearLocked(); closed = true } }
    fun invalidateIfStale() {
        val captured = synchronized(monitor) { owner } ?: return
        if (!retained(captured)) synchronized(monitor) { if (owner === captured) clearLocked(EnterpriseAccessError.STALE) }
    }

    private fun begin(renderedEpoch: Long, generation: Long): Owner? {
        invalidateIfStale()
        return synchronized(monitor) {
            val before = owner ?: return@synchronized null
            if (closed || mutable.value.busy || before.epoch != renderedEpoch || before.token != generation ||
                fence.epoch != renderedEpoch || fence.backgroundEpoch != before.background) return@synchronized null
            val request = Owner(next(), before.context, before.epoch, before.background, before.account)
            owner = request
            mutable.value = mutable.value.copy(generation = request.token, busy = true, captureInvalid = false, saveFailed = false,
                lastSaved = null, sendFailed = false, attachmentRefusal = null, receiptNotSaved = false, lastAttached = null)
            request
        }
    }

    /** The delivery proof may refresh the session: the fresh lease replaces the earlier one at once (EDR-335). */
    private suspend fun account(request: Owner, delivery: Boolean = false): EnterpriseQueueAccountLease {
        val account = if (delivery) source.deliveryAccount() else source.account()
        if (!account.identity.owns(request.context.partition) || account.epoch != request.epoch || account.background != request.background)
            throw EnterpriseLocalDenied()
        synchronized(monitor) { if (owner === request) request.account = account }
        return account
    }

    private suspend fun publish(request: Owner, saved: String? = null, sendFailed: Boolean = false, attached: String? = null,
        receiptNotSaved: Boolean = false, receiptRefusal: AttachmentLocalFailure? = null) {
        val expenses = service.expenses(account(request), request.context.partition)
        val photos = receipts?.takeIf { it.enabled }?.let {
            try { it.byOwner(request.context.partition) }
            catch (cancelled: CancellationException) { throw cancelled }
            catch (denied: EnterpriseLocalDenied) { throw denied }
            catch (_: Exception) { null }
        }
        if (!retained(request)) { invalidateIfStale(); return }
        synchronized(monitor) {
            if (owner !== request || !viewCurrent(request)) return
            val open = mutable.value.openExpenseId?.takeIf { id -> expenses.any { it.fact.expenseId == id } }
            mutable.value = mutable.value.copy(busy = false, loaded = true, expenses = expenses, openExpenseId = open, lastSaved = saved,
                sendFailed = sendFailed, error = null, unlockRequired = false, unrecoverable = false, attachments = photos ?: emptyMap(),
                attachmentsUnavailable = receipts?.enabled == true && photos == null, lastAttached = attached, receiptNotSaved = receiptNotSaved,
                attachmentRefusal = receiptRefusal)
        }
    }

    private fun failed(request: Owner, failure: Exception, write: Boolean) {
        val sourceCurrent = retained(request)
        synchronized(monitor) {
            if (owner !== request || closed) return
            val current = mutable.value
            mutable.value = when {
                failure is EnterpriseLocalDenied || !sourceCurrent || !viewCurrent(request) -> { clearLocked(EnterpriseAccessError.STALE); return }
                failure is ExpenseCaptureInvalid -> current.copy(busy = false, captureInvalid = true)
                failure is ReceiptSaveRefused -> current.copy(busy = false, attachmentRefusal = failure.failure)
                failure is EnterpriseUnlockRequired || failure is EnterpriseVaultUnrecoverable -> current.copy(busy = false, loaded = false,
                    expenses = emptyList(), openExpenseId = null, error = EnterpriseAccessError.UNAVAILABLE,
                    unlockRequired = failure is EnterpriseUnlockRequired, unrecoverable = failure is EnterpriseVaultUnrecoverable)
                write -> current.copy(busy = false, saveFailed = true)
                else -> current.copy(busy = false, loaded = false, expenses = emptyList(), openExpenseId = null, error = EnterpriseAccessError.UNAVAILABLE)
            }
        }
    }

    private suspend fun perform(request: Owner, write: Boolean, block: suspend () -> Unit) {
        try { currentCoroutineContext().ensureActive(); block() }
        catch (cancelled: CancellationException) { synchronized(monitor) { if (owner === request) clearLocked() }; throw cancelled }
        catch (failure: Exception) { failed(request, failure, write) }
    }

    /** Reads the protected expenses of the exact partition; no network. */
    suspend fun open(context: EnterpriseContextSummary, renderedEpoch: Long) {
        invalidateIfStale()
        val request = synchronized(monitor) {
            if (closed || fence.epoch != renderedEpoch) return
            val created = Owner(next(), context, renderedEpoch, fence.backgroundEpoch)
            owner = created
            mutable.value = ExpenseViewState(generation = created.token, epoch = created.epoch, context = context, busy = true)
            created
        }
        perform(request, write = false) { publish(request) }
    }

    suspend fun reload(renderedEpoch: Long, generation: Long) {
        val request = begin(renderedEpoch, generation) ?: return
        perform(request, write = false) { publish(request) }
    }

    /**
     * Saves the expense, then — in a second protected write that needs the first — its optional receipt with target
     * `EXPENSE` (EDR-338). A refused or failed receipt write leaves the saved expense and says the receipt was not saved.
     */
    suspend fun record(choice: EnterpriseAssetChoice, form: ExpenseForm, receipt: ReceiptDraft?, renderedEpoch: Long, generation: Long) {
        val request = begin(renderedEpoch, generation) ?: return
        perform(request, write = true) {
            val saved = service.record(account(request), request.context, choice, form)
            var attached: String? = null
            var notSaved = false
            var refusal: AttachmentLocalFailure? = null
            if (receipt != null) {
                try {
                    val photos = receipts?.takeIf { it.enabled } ?: throw ReceiptSaveRefused(AttachmentLocalFailure.DISABLED)
                    photos.save(request.context.partition, saved.operationId, AttachmentTarget(DocumentTargetKind.EXPENSE, saved.expenseId), receipt)
                    attached = saved.operationId
                } catch (cancelled: CancellationException) { throw cancelled }
                catch (denied: EnterpriseLocalDenied) { throw denied }
                catch (local: ReceiptSaveRefused) { notSaved = true; refusal = local.failure }
                catch (_: Exception) { notSaved = true }
            }
            if (!retained(request)) { invalidateIfStale(); return@perform }
            synchronized(monitor) { if (owner === request) mutable.value = mutable.value.copy(openExpenseId = saved.expenseId) }
            publish(request, saved = saved.operationId, attached = attached, receiptNotSaved = notSaved, receiptRefusal = refusal)
        }
    }

    /** EMP-DOC-004: a receipt added later to a saved expense of the visible partition. */
    suspend fun attach(expenseId: String, draft: ReceiptDraft, renderedEpoch: Long, generation: Long) {
        val request = begin(renderedEpoch, generation) ?: return
        perform(request, write = true) {
            val expense = mutable.value.expenses.firstOrNull { it.fact.expenseId == expenseId } ?: throw ReceiptSaveRefused(AttachmentLocalFailure.UNKNOWN_OPERATION)
            val photos = receipts?.takeIf { it.enabled } ?: throw ReceiptSaveRefused(AttachmentLocalFailure.DISABLED)
            photos.save(request.context.partition, expense.fact.operationId, AttachmentTarget(DocumentTargetKind.EXPENSE, expenseId), draft)
            if (!retained(request)) { invalidateIfStale(); return@perform }
            publish(request, attached = expense.fact.operationId)
        }
    }

    /** Online only: one delivery attempt of this partition's queue, then a local re-read. */
    suspend fun sendNow(renderedEpoch: Long, generation: Long) {
        val request = begin(renderedEpoch, generation) ?: return
        perform(request, write = false) {
            val failedSend = try { deliver(account(request, delivery = true), request.context.partition); false }
            catch (cancelled: CancellationException) { throw cancelled }
            catch (denied: EnterpriseLocalDenied) { throw denied }
            catch (_: Exception) { true }
            if (!retained(request)) { invalidateIfStale(); return@perform }
            publish(request, sendFailed = failedSend)
        }
    }

    fun openExpense(expenseId: String?, renderedEpoch: Long, generation: Long) {
        invalidateIfStale()
        synchronized(monitor) {
            val current = owner ?: return
            if (closed || current.epoch != renderedEpoch || current.token != generation || fence.epoch != renderedEpoch) return
            if (expenseId != null && mutable.value.expenses.none { it.fact.expenseId == expenseId }) return
            mutable.value = mutable.value.copy(openExpenseId = expenseId)
        }
    }
}
