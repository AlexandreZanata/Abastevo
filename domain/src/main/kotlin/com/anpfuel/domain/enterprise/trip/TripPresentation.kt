package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncProjection
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import java.time.Instant

/*
 * What the trip screen may state (EMP-TRIP-005, B-BR-EMP-379, EDR-245): facts from the immutable operations,
 * each record's sync state from its own retained queue entry, and only the review state a receipt proves.
 */

/** One timeline or submission record with its own sync state and, when final, its receipt code and instant. */
class TripRecordView(val fact: TripOperationFact, val sync: EnterpriseSyncStatus, val code: String?, val receivedAt: Instant?) {
    override fun toString(): String = "EnterpriseTripRecordView(redacted)"
}

/** The review line (BR-REV-002): nothing before "ready to send"; SUBMITTED only from the submission's ACCEPTED receipt. */
enum class TripReviewLine { NOT_READY, READY_TO_SEND, AWAITING_APPROVAL, SUBMISSION_NEEDS_REVIEW, SUBMISSION_REFUSED }

class TripView(
    val trip: LocalTrip,
    val start: TripRecordView,
    events: List<TripRecordView>,
    val finish: TripRecordView?,
    val submission: TripRecordView?,
    val review: TripReviewLine,
    /** The start needs review and later records still wait on it. */
    val waitingForStartReview: Boolean,
) {
    private val values = events.toList()
    /** Logbook events in occurrence order (B-BR-EMP-324). */
    val events: List<TripRecordView> get() = values.toList()
    override fun toString(): String = "EnterpriseTripView(redacted)"
}

object TripPresentation {
    private fun record(entries: Map<String, EnterpriseQueueEntry>, operationId: String, trip: LocalTrip): TripRecordView {
        val entry = requireNotNull(entries[operationId]) { "Missing trip record" }
        require(entry.operation.operationId == operationId) { "Foreign trip record" }
        val fact = TripOperations.read(entry.operation)
        val owner = when (fact) {
            is TripOperationFact.Start -> fact.tripId.takeIf { fact.assetId == trip.assetId && operationId == trip.startOperationId }
            is TripOperationFact.Event -> fact.tripId.takeIf { operationId in trip.eventOperationIds }
            is TripOperationFact.Finish -> fact.tripId.takeIf { operationId == trip.finishOperationId }
            is TripOperationFact.Submit -> fact.tripId.takeIf { operationId == trip.submitOperationId }
        }
        require(owner == trip.tripId) { "Foreign trip record" }
        val receipt = entry.acknowledgement
        return TripRecordView(fact, EnterpriseSyncProjection.status(EnterpriseSyncProjection.item(entry)), receipt?.code, receipt?.receivedAt)
    }

    /** Every operation of [trip] must have its retained entry; anything else is refused, never guessed. */
    fun view(trip: LocalTrip, entries: Map<String, EnterpriseQueueEntry>): TripView {
        val start = record(entries, trip.startOperationId, trip)
        val events = trip.eventOperationIds.map { record(entries, it, trip) }.sortedWith(compareBy(
            { it.fact.declared.occurredAt }, { (it.fact as TripOperationFact.Event).eventId }))
        val finish = trip.finishOperationId?.let { record(entries, it, trip) }
        val submission = trip.submitOperationId?.let { record(entries, it, trip) }
        val review = when (submission?.sync) {
            null -> TripReviewLine.NOT_READY
            EnterpriseSyncStatus.ACCEPTED -> TripReviewLine.AWAITING_APPROVAL
            EnterpriseSyncStatus.NEEDS_REVIEW -> TripReviewLine.SUBMISSION_NEEDS_REVIEW
            EnterpriseSyncStatus.REJECTED -> TripReviewLine.SUBMISSION_REFUSED
            else -> TripReviewLine.READY_TO_SEND
        }
        val unsent = setOf(EnterpriseSyncStatus.SAVED_ON_DEVICE, EnterpriseSyncStatus.SENDING, EnterpriseSyncStatus.SENT)
        val waiting = start.sync == EnterpriseSyncStatus.NEEDS_REVIEW && (events + listOfNotNull(finish, submission)).any { it.sync in unsent }
        return TripView(trip, start, events, finish, submission, review, waiting)
    }

    /** Newest start first; equal instants by trip identifier. */
    fun views(book: TripBook, entries: Map<String, EnterpriseQueueEntry>): List<TripView> = book.trips.map { view(it, entries) }
        .sortedWith(compareByDescending<TripView> { it.trip.startedAt }.thenBy { it.trip.tripId })
}
