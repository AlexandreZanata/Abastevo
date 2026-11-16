package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.attachment.AttachmentLabel
import com.anpfuel.domain.enterprise.attachment.AttachmentView
import com.anpfuel.domain.enterprise.expense.ExpenseView
import com.anpfuel.domain.enterprise.fueling.FuelingView
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import java.time.Instant

/*
 * EMP-TRIP-020 — the closing conference (B-BR-EMP-522..526, EDR-339..341). Before the driver marks a finished trip
 * ready to send, it lists what this device recorded for that trip: the trip's own departure and final readings and
 * the fuelings and expenses saved on this device, in the trip's organization, for the trip's vehicle, between its
 * start and its finish. Each item keeps its own record state (from its own receipt) and its own evidence state (from
 * its own photos). Device-local only: nothing here is a server link, a cost total, an approval or a verification.
 */

enum class ClosingItemKind { DEPARTURE_READING, FINAL_READING, FUELING, EXPENSE }

/** The record's state, read only from its own retained receipt. */
enum class ClosingRecordState { SAVED_ON_DEVICE, SENDING, ACCEPTED, NEEDS_REVIEW, REFUSED }

/**
 * The evidence of one item: no photo or only failed ones ([MISSING]), a photo saved on the device or still being
 * transferred or checked ([PENDING]), every photo stored by the company ([AVAILABLE]), or an attachment index this
 * device cannot read ([UNREADABLE], never shown as available). "Available" means stored, never verified or certified.
 */
enum class ClosingEvidenceState { MISSING, PENDING, AVAILABLE, UNREADABLE }

class ClosingItem(
    val kind: ClosingItemKind,
    /** The owning operation (the reading's trip record, the fueling's or the expense's submit). */
    val operationId: String,
    /** The record the photos document: the trip operation, the fueling or the expense. */
    val recordId: String,
    val occurredAt: Instant,
    val record: ClosingRecordState,
    val code: String?,
    val evidence: ClosingEvidenceState,
    val photos: Int,
    /** The amount entered on that record, shown per item only; the conference never adds amounts up. */
    val paidCentavos: Long?,
) {
    override fun toString(): String = "EnterpriseClosingItem(redacted)"
}

/** What the submission action may do: queue the first `trip.submit` (revision 1), or nothing (already queued, or not finished). */
enum class ClosingSubmission { NOT_FINISHED, READY_TO_MARK, ALREADY_MARKED }

class TripClosing(val tripId: String, items: List<ClosingItem>, val submission: ClosingSubmission, val revision: Long?,
    /** Fuelings or expenses of this device could not be read: the list may be incomplete and says so. */
    val relatedUnavailable: Boolean) {
    private val values = items.toList()
    val items: List<ClosingItem> get() = values.toList()
    override fun toString(): String = "EnterpriseTripClosing(redacted)"
}

object TripClosings {
    /** The first submission names review revision 1 (EDR-220); a resubmission needs the returned revision (retained). */
    const val FIRST_REVISION = 1L

    fun record(sync: EnterpriseSyncStatus): ClosingRecordState = when (sync) {
        EnterpriseSyncStatus.SAVED_ON_DEVICE -> ClosingRecordState.SAVED_ON_DEVICE
        EnterpriseSyncStatus.SENDING, EnterpriseSyncStatus.SENT -> ClosingRecordState.SENDING
        EnterpriseSyncStatus.ACCEPTED -> ClosingRecordState.ACCEPTED
        EnterpriseSyncStatus.NEEDS_REVIEW -> ClosingRecordState.NEEDS_REVIEW
        EnterpriseSyncStatus.REJECTED -> ClosingRecordState.REFUSED
    }

    /** One item's evidence from its own photos only; a photo of another record never counts (B-BR-EMP-524). */
    fun evidence(photos: List<AttachmentView>?): ClosingEvidenceState {
        if (photos == null) return ClosingEvidenceState.UNREADABLE
        val usable = photos.filter { it.label != AttachmentLabel.FILE_MISSING && it.label != AttachmentLabel.REFUSED }
        return when {
            usable.isEmpty() -> ClosingEvidenceState.MISSING
            usable.all { it.label == AttachmentLabel.VERIFIED } -> ClosingEvidenceState.AVAILABLE
            else -> ClosingEvidenceState.PENDING
        }
    }

    /**
     * The conference of [trip]. [attachments] are the partition's photos by owning operation, null when the index is
     * unreadable; [fuelings] and [expenses] are the same partition's own records, null when unreadable. Records of
     * another vehicle or outside the trip's interval are not this trip's; duplicates count once.
     */
    fun of(trip: TripView, attachments: Map<String, List<AttachmentView>>?, fuelings: List<FuelingView>?, expenses: List<ExpenseView>?): TripClosing {
        val local = trip.trip
        fun photos(operationId: String, recordId: String): List<AttachmentView>? =
            attachments?.let { byOwner -> byOwner[operationId].orEmpty().filter { it.attachment.target.id == recordId } }
        fun during(at: Instant): Boolean = !at.isBefore(local.startedAt) && (local.finishedAt == null || !at.isAfter(local.finishedAt))
        val items = mutableListOf<ClosingItem>()
        fun reading(kind: ClosingItemKind, view: TripRecordView, operationId: String) {
            // A reading photo documents the reading record of that operation (EMP-DOC-004, METER_READING).
            val readingId = when (val fact = view.fact) {
                is TripOperationFact.Start -> (fact.evidence as? TripEvidence.Reading)?.readingId
                is TripOperationFact.Finish -> (fact.evidence as? TripEvidence.Reading)?.readingId
                else -> null
            }
            val owned = attachments?.let { byOwner -> byOwner[operationId].orEmpty().filter { readingId != null && it.attachment.target.id == readingId } }
            items += ClosingItem(kind, operationId, readingId ?: operationId, view.fact.declared.occurredAt, record(view.sync), view.code,
                evidence(owned), owned?.size ?: 0, null)
        }
        reading(ClosingItemKind.DEPARTURE_READING, trip.start, local.startOperationId)
        trip.finish?.let { reading(ClosingItemKind.FINAL_READING, it, requireNotNull(local.finishOperationId)) }
        fuelings.orEmpty().filter { it.fact.assetId == local.assetId && during(it.fact.declared.occurredAt) }.distinctBy { it.fact.fuelingId }.forEach {
            val owned = photos(it.fact.operationId, it.fact.fuelingId)
            items += ClosingItem(ClosingItemKind.FUELING, it.fact.operationId, it.fact.fuelingId, it.fact.declared.occurredAt, record(it.sync), it.code,
                evidence(owned), owned?.size ?: 0, it.fact.amounts.paid)
        }
        expenses.orEmpty().filter { it.fact.assetId == local.assetId && during(it.fact.declared.occurredAt) }.distinctBy { it.fact.expenseId }.forEach {
            val owned = photos(it.fact.operationId, it.fact.expenseId)
            items += ClosingItem(ClosingItemKind.EXPENSE, it.fact.operationId, it.fact.expenseId, it.fact.declared.occurredAt, record(it.sync), it.code,
                evidence(owned), owned?.size ?: 0, it.fact.paidCentavos)
        }
        val submission = when {
            local.submitOperationId != null -> ClosingSubmission.ALREADY_MARKED
            local.finishOperationId == null -> ClosingSubmission.NOT_FINISHED
            else -> ClosingSubmission.READY_TO_MARK
        }
        val ordered = items.sortedWith(compareBy<ClosingItem> { it.kind != ClosingItemKind.DEPARTURE_READING }
            .thenBy { it.kind == ClosingItemKind.FINAL_READING }.thenBy { it.occurredAt }.thenBy { it.recordId })
        return TripClosing(local.tripId, ordered, submission, if (submission == ClosingSubmission.READY_TO_MARK) FIRST_REVISION else null,
            fuelings == null || expenses == null)
    }
}
