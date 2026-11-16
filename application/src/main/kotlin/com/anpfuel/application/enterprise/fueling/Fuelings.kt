package com.anpfuel.application.enterprise.fueling

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
import com.anpfuel.application.enterprise.trip.TripCapture
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseValueException
import com.anpfuel.domain.enterprise.EnterpriseValues
import com.anpfuel.domain.enterprise.ExactRecord
import com.anpfuel.domain.enterprise.attachment.AttachmentLocalFailure
import com.anpfuel.domain.enterprise.attachment.AttachmentTarget
import com.anpfuel.domain.enterprise.attachment.AttachmentView
import com.anpfuel.domain.enterprise.document.DocumentTargetKind
import com.anpfuel.domain.enterprise.fleet.EnterpriseAssetChoice
import com.anpfuel.domain.enterprise.fueling.*
import com.anpfuel.domain.enterprise.meter.MeterDomainException
import com.anpfuel.domain.enterprise.meter.MeterPlausibility
import com.anpfuel.domain.enterprise.meter.MeterPoint
import com.anpfuel.domain.enterprise.meter.MeterPresence
import com.anpfuel.domain.enterprise.meter.MeterSeriesEntry
import com.anpfuel.domain.enterprise.meter.MeterValidation
import com.anpfuel.domain.enterprise.meter.MeterValues
import com.anpfuel.domain.enterprise.meter.ReadingFlag
import com.anpfuel.domain.enterprise.meter.ReadingState
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.trip.TripDeclarations
import java.time.Instant
import java.time.ZoneId
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/*
 * EMP-FUEL-011 — the offline fueling producer (B-BR-EMP-455..460, EDR-294..297). The operation is the whole record;
 * acceptance is only ever read from its own retained receipt; a receipt photo is a separate EMP-DOC-004 capture.
 */

/** The partition's retained `fueling.submit` entries, read in one protected snapshot (EDR-295). */
fun interface EnterpriseFuelingStore {
    suspend fun read(attempt: EnterpriseQueueAttempt): List<EnterpriseQueueEntry>
}

/** The registered `fueling.submit` producer: it admits only an operation the device decoder reads back (B-BR-EMP-455). */
class EnterpriseFuelingProducer : EnterpriseQueueProducer {
    override fun descriptor(): EnterpriseQueueProducerDescriptor = EnterpriseQueueProducerDescriptor(FuelingOperations.TYPE)
    override fun save(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation) {
        require(operation.type == FuelingOperations.TYPE)
        // Inside the protected write: a fueling that the intake parser would not read is never queued.
        FuelingOperations.read(operation)
    }
    /** The receipt is the truth; no local index changes (EDR-295). */
    override fun acknowledge(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
        ack: OperationAcknowledgement) = Unit
    override fun toString(): String = "EnterpriseFuelingProducer(redacted)"
}

/** What the driver entered, as typed (EDR-296); the service converts it exactly or refuses it. */
class FuelingForm(
    val product: FuelProduct,
    val quantity: String,
    val paidTotal: String,
    val discount: String? = null,
    val unitPrice: String? = null,
    val fill: FillLevel,
    val payer: Payer,
    val payment: PaymentMethod,
    val reading: TripCapture,
    val placeName: String? = null,
) {
    override fun toString(): String = "FuelingForm(redacted)"
}

/** The entered values do not fit their profile; nothing was saved. */
class FuelingCaptureInvalid : IllegalArgumentException("Fueling capture invalid")
/** The selected context has no employee identifier; confirm the organization again. */
class FuelingResponsibleUnknown : IllegalStateException("Fueling responsible unknown")

/** Warnings shown inline, never a block (BR-FUEL-017): arithmetic and the METER-003 reading check. */
class FuelingWarnings(val arithmeticMismatch: Boolean, val reading: List<ReadingFlag>) {
    override fun toString(): String = "FuelingWarnings(redacted)"
}

/** One saved fueling: its operation identifier and the fueling (record) identifier. */
class FuelingSaved(val operationId: String, val fuelingId: String) {
    override fun toString(): String = "FuelingSaved(redacted)"
}

/** Offline fueling actions of one partition; every write is durable before it returns (BR-OFFL-002). */
class EnterpriseFuelingService(
    private val source: EnterpriseQueueSource,
    private val save: suspend (EnterprisePartition, EnterpriseOperation) -> Boolean,
    private val store: EnterpriseFuelingStore,
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

    suspend fun fuelings(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): List<FuelingView> {
        val attempt = attempt(account, partition)
        val entries = store.read(attempt)
        attempt.assertCurrent()
        return FuelingPresentation.views(entries)
    }

    private fun input(context: EnterpriseContextSummary, choice: EnterpriseAssetChoice, form: FuelingForm): FuelingSubmitInput {
        val responsible = context.employeeId ?: throw FuelingResponsibleUnknown()
        val quantity = FuelingEntry.quantity(form.quantity) ?: throw FuelingCaptureInvalid()
        val paid = FuelingEntry.money(form.paidTotal) ?: throw FuelingCaptureInvalid()
        val discount = form.discount?.takeIf { it.isNotBlank() }?.let { FuelingEntry.money(it) ?: throw FuelingCaptureInvalid() }
        val price = form.unitPrice?.takeIf { it.isNotBlank() }?.let { FuelingEntry.unitPrice(it) ?: throw FuelingCaptureInvalid() }
        val (reading, reason) = when (val capture = form.reading) {
            is TripCapture.Absent -> null to capture.reason
            is TripCapture.Reading -> {
                val value = try { MeterValues.parse(capture.meter.kind, capture.form, capture.raw.trim(), capture.minutes?.trim(), capture.meter.modulus) }
                    catch (_: MeterDomainException) { throw FuelingCaptureInvalid() }
                FuelingReadingInput(capture.meter.meterId, capture.meter.segmentId, newId(), value.kind.name, value.units.toString()) to null
            }
        }
        return FuelingSubmitInput(choice.assetId, responsible, form.product.name, form.product.unit.name, quantity, paid, discount, price, null,
            form.fill.name, form.payer.name, form.payment.name, reading, reason,
            form.placeName?.takeIf { it.isNotBlank() }?.let { PlaceKind.NOT_FOUND.name }, form.placeName?.takeIf { it.isNotBlank() })
    }

    /** Builds and durably queues one `fueling.submit` in the context's own partition (B-BR-EMP-456). */
    suspend fun record(account: EnterpriseQueueAccountLease, context: EnterpriseContextSummary, choice: EnterpriseAssetChoice,
        form: FuelingForm): FuelingSaved {
        val partition = context.partition
        if (choice.organizationId != partition.organizationId) throw EnterpriseLocalDenied()
        val attempt = attempt(account, partition)
        if (attempt.enrollment.target.membershipId != context.membershipId) throw EnterpriseLocalDenied()
        val input = input(context, choice, form)
        val operationId = newId()
        val fuelingId = newId()
        val declared = TripDeclarations.at(wallMillis(), zone())
        val envelope = FuelingEnvelope(SyncBinding.of(partition.ownerAccount, partition.origin, partition.organizationId),
            attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId, operationId, declared)
        val operation = try { FuelingOperations.submit(envelope, fuelingId, input) } catch (_: FuelingOperationException) { throw FuelingCaptureInvalid() }
        save(partition, operation)
        return FuelingSaved(operationId, fuelingId)
    }

    /** Arithmetic (BR-MONEY-005) and METER-003 against the readings of this device's own fuelings; never a refusal. */
    fun warnings(form: FuelingForm, previous: List<FuelingView>): FuelingWarnings {
        val quantity = FuelingEntry.quantity(form.quantity)?.toLong()
        val paid = FuelingEntry.money(form.paidTotal)?.toLong()
        val discount = form.discount?.takeIf { it.isNotBlank() }?.let { FuelingEntry.money(it)?.toLong() ?: return FuelingWarnings(false, emptyList()) } ?: 0L
        val price = form.unitPrice?.takeIf { it.isNotBlank() }?.let { FuelingEntry.unitPrice(it)?.toLong() }
        val arithmetic = if (quantity != null && paid != null && price != null) try {
            // The financial-v1 profile; KWH follows the same integer formula (EDR-176).
            val unit = if (form.product.unit == FuelUnit.M3) "M3" else "L"
            EnterpriseValues.reconcile(ExactRecord(paid, quantity, price, discount, unit)).warning == "ARITHMETIC_MISMATCH"
        } catch (_: EnterpriseValueException) { false } else false
        return FuelingWarnings(arithmetic, readingFlags(form.reading, previous))
    }

    private fun readingFlags(capture: TripCapture, previous: List<FuelingView>): List<ReadingFlag> {
        if (capture !is TripCapture.Reading) return emptyList()
        val value = try { MeterValues.parse(capture.meter.kind, capture.form, capture.raw.trim(), capture.minutes?.trim(), capture.meter.modulus) }
            catch (_: MeterDomainException) { return emptyList() }
        val series = previous.mapNotNull { view ->
            val evidence = view.fact.evidence as? FuelingEvidence.Reading ?: return@mapNotNull null
            if (evidence.meterId != capture.meter.meterId || evidence.segmentId != capture.meter.segmentId || evidence.value.kind != value.kind) null
            else MeterSeriesEntry(MeterPoint(evidence.readingId, evidence.value.units, view.fact.declared.occurredAt, view.fact.declared.occurredAt), ReadingState.VALID)
        }
        if (series.isEmpty()) return emptyList()
        val at = Instant.ofEpochMilli(wallMillis())
        if (!at.isAfter(series.maxOf { it.point.occurredAt })) return emptyList()
        return try {
            MeterValidation.check(MeterPlausibility.DEFAULT, value.kind, series, MeterPoint("candidate", value.units, at, at)).flags
                .filter { it != ReadingFlag.FUTURE_INSTANT }
        } catch (_: MeterDomainException) { emptyList() }
    }
    override fun toString(): String = "EnterpriseFuelingService(redacted)"
}

data class FuelingViewState(
    val generation: Long = 0,
    val epoch: Long? = null,
    val context: EnterpriseContextSummary? = null,
    val busy: Boolean = false,
    /** The protected queue was read; an empty list then means no fueling on this device. */
    val loaded: Boolean = false,
    val fuelings: List<FuelingView> = emptyList(),
    val openFuelingId: String? = null,
    val captureInvalid: Boolean = false,
    /** The latest write failed; nothing was saved (BR-OFFL-002). */
    val saveFailed: Boolean = false,
    val responsibleUnknown: Boolean = false,
    /** The operation identifier of the latest durably saved fueling, for one polite announcement. */
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
    /** The fueling was saved but its receipt was not (a failed second write); the photo stays in memory for another attempt. */
    val receiptNotSaved: Boolean = false,
    /** The owning operation of the latest durably saved receipt. */
    val lastAttached: String? = null,
) {
    val openFueling: FuelingView? get() = openFuelingId?.let { id -> fuelings.firstOrNull { it.fact.fuelingId == id } }
    override fun toString(): String = "FuelingViewState(redacted)"
}

/** One visible fueling owner per screen, bound to account, epoch and background lease like the trip owner. */
class FuelingFlow(
    private val source: EnterpriseQueueSource,
    private val service: EnterpriseFuelingService,
    private val fence: EnterpriseSessionFence,
    private val receipts: EnterpriseReceipts? = null,
    private val deliver: suspend (EnterpriseQueueAccountLease, EnterprisePartition) -> Unit,
) {
    private val mutable = MutableStateFlow(FuelingViewState())
    val state: StateFlow<FuelingViewState> = mutable.asStateFlow()
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
        mutable.value = FuelingViewState(generation = next(), error = error)
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
                responsibleUnknown = false, lastSaved = null, sendFailed = false, attachmentRefusal = null, receiptNotSaved = false, lastAttached = null)
            request
        }
    }

    private suspend fun account(request: Owner, delivery: Boolean = false): EnterpriseQueueAccountLease {
        val account = if (delivery) source.deliveryAccount() else source.account()
        if (!account.identity.owns(request.context.partition) || account.epoch != request.epoch || account.background != request.background)
            throw EnterpriseLocalDenied()
        synchronized(monitor) { if (owner === request) request.account = account }
        return account
    }

    private suspend fun publish(request: Owner, saved: String? = null, sendFailed: Boolean = false, attached: String? = null,
        receiptNotSaved: Boolean = false, receiptRefusal: AttachmentLocalFailure? = null) {
        val fuelings = service.fuelings(account(request), request.context.partition)
        val photos = receipts?.takeIf { it.enabled }?.let {
            try { it.byOwner(request.context.partition) }
            catch (cancelled: CancellationException) { throw cancelled }
            catch (denied: EnterpriseLocalDenied) { throw denied }
            catch (_: Exception) { null }
        }
        if (!retained(request)) { invalidateIfStale(); return }
        synchronized(monitor) {
            if (owner !== request || !viewCurrent(request)) return
            val open = mutable.value.openFuelingId?.takeIf { id -> fuelings.any { it.fact.fuelingId == id } }
            mutable.value = mutable.value.copy(busy = false, loaded = true, fuelings = fuelings, openFuelingId = open, lastSaved = saved,
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
                failure is FuelingCaptureInvalid -> current.copy(busy = false, captureInvalid = true)
                failure is FuelingResponsibleUnknown -> current.copy(busy = false, responsibleUnknown = true)
                failure is ReceiptSaveRefused -> current.copy(busy = false, attachmentRefusal = failure.failure)
                failure is EnterpriseUnlockRequired || failure is EnterpriseVaultUnrecoverable -> current.copy(busy = false, loaded = false,
                    fuelings = emptyList(), openFuelingId = null, error = EnterpriseAccessError.UNAVAILABLE,
                    unlockRequired = failure is EnterpriseUnlockRequired, unrecoverable = failure is EnterpriseVaultUnrecoverable)
                write -> current.copy(busy = false, saveFailed = true)
                else -> current.copy(busy = false, loaded = false, fuelings = emptyList(), openFuelingId = null, error = EnterpriseAccessError.UNAVAILABLE)
            }
        }
    }

    private suspend fun perform(request: Owner, write: Boolean, block: suspend () -> Unit) {
        try { currentCoroutineContext().ensureActive(); block() }
        catch (cancelled: CancellationException) { synchronized(monitor) { if (owner === request) clearLocked() }; throw cancelled }
        catch (failure: Exception) { failed(request, failure, write) }
    }

    /** Reads the protected fuelings of the exact partition; no network. */
    suspend fun open(context: EnterpriseContextSummary, renderedEpoch: Long) {
        invalidateIfStale()
        val request = synchronized(monitor) {
            if (closed || fence.epoch != renderedEpoch) return
            val created = Owner(next(), context, renderedEpoch, fence.backgroundEpoch)
            owner = created
            mutable.value = FuelingViewState(generation = created.token, epoch = created.epoch, context = context, busy = true)
            created
        }
        perform(request, write = false) { publish(request) }
    }

    suspend fun reload(renderedEpoch: Long, generation: Long) {
        val request = begin(renderedEpoch, generation) ?: return
        perform(request, write = false) { publish(request) }
    }

    /**
     * Saves the fueling, then — in a second protected write that needs the first — its optional receipt with target
     * `FUELING` (EDR-297). A refused or failed receipt write leaves the saved fueling and says the receipt was not saved.
     */
    suspend fun record(choice: EnterpriseAssetChoice, form: FuelingForm, receipt: ReceiptDraft?, renderedEpoch: Long, generation: Long) {
        val request = begin(renderedEpoch, generation) ?: return
        perform(request, write = true) {
            val saved = service.record(account(request), request.context, choice, form)
            var attached: String? = null
            var notSaved = false
            var refusal: AttachmentLocalFailure? = null
            if (receipt != null) {
                try {
                    val photos = receipts?.takeIf { it.enabled } ?: throw ReceiptSaveRefused(AttachmentLocalFailure.DISABLED)
                    photos.save(request.context.partition, saved.operationId, AttachmentTarget(DocumentTargetKind.FUELING, saved.fuelingId), receipt)
                    attached = saved.operationId
                } catch (cancelled: CancellationException) { throw cancelled }
                catch (denied: EnterpriseLocalDenied) { throw denied }
                catch (local: ReceiptSaveRefused) { notSaved = true; refusal = local.failure }
                catch (_: Exception) { notSaved = true }
            }
            if (!retained(request)) { invalidateIfStale(); return@perform }
            synchronized(monitor) { if (owner === request) mutable.value = mutable.value.copy(openFuelingId = saved.fuelingId) }
            publish(request, saved = saved.operationId, attached = attached, receiptNotSaved = notSaved, receiptRefusal = refusal)
        }
    }

    /** EMP-DOC-004: a receipt added later to a saved fueling of the visible partition. */
    suspend fun attach(fuelingId: String, draft: ReceiptDraft, renderedEpoch: Long, generation: Long) {
        val request = begin(renderedEpoch, generation) ?: return
        perform(request, write = true) {
            val fueling = mutable.value.fuelings.firstOrNull { it.fact.fuelingId == fuelingId } ?: throw ReceiptSaveRefused(AttachmentLocalFailure.UNKNOWN_OPERATION)
            val photos = receipts?.takeIf { it.enabled } ?: throw ReceiptSaveRefused(AttachmentLocalFailure.DISABLED)
            photos.save(request.context.partition, fueling.fact.operationId, AttachmentTarget(DocumentTargetKind.FUELING, fuelingId), draft)
            if (!retained(request)) { invalidateIfStale(); return@perform }
            publish(request, attached = fueling.fact.operationId)
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

    fun openFueling(fuelingId: String?, renderedEpoch: Long, generation: Long) {
        invalidateIfStale()
        synchronized(monitor) {
            val current = owner ?: return
            if (closed || current.epoch != renderedEpoch || current.token != generation || fence.epoch != renderedEpoch) return
            if (fuelingId != null && mutable.value.fuelings.none { it.fact.fuelingId == fuelingId }) return
            mutable.value = mutable.value.copy(openFuelingId = fuelingId)
        }
    }

    fun warnings(form: FuelingForm): FuelingWarnings = service.warnings(form, mutable.value.fuelings)
}
