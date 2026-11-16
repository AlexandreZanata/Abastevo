package com.anpfuel.data.enterprise.tracking

import com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.tracking.EnterpriseTrackingProducers
import com.anpfuel.application.enterprise.tracking.EnterpriseTrackingSnapshot
import com.anpfuel.application.enterprise.tracking.EnterpriseTrackingStore
import com.anpfuel.application.enterprise.trip.EnterpriseTripProducers
import com.anpfuel.data.enterprise.ProtectedEnterprisePersistence
import com.anpfuel.data.enterprise.VaultEntry
import com.anpfuel.data.enterprise.VaultKind
import com.anpfuel.data.enterprise.VaultQueueChange
import com.anpfuel.data.enterprise.VaultSnapshot
import com.anpfuel.data.enterprise.protectedqueue.ProtectedQueueCodec
import com.anpfuel.domain.enterprise.tracking.SessionBuffer
import com.anpfuel.domain.enterprise.tracking.TrackingBook
import com.anpfuel.domain.enterprise.tracking.TrackingBufferBook
import com.anpfuel.domain.enterprise.tracking.TrackingCheckpoint
import com.anpfuel.domain.enterprise.tracking.TrackingPolicyRead
import com.anpfuel.domain.enterprise.trip.TripBook

/**
 * The EMP-GPS-002 local tracking storage (EDR-322/324): the sealed `tracking-book` RECORD of the exact partition's
 * protected generation, its trip book and the retained queue entries both name, read in one snapshot through the
 * attempt's custody, enrollment and lease checks. The policy cache and the checkpoint are written without an operation
 * in one locked read-modify-write; transitions are written by the `gps.session.*` producers in the queue's own write.
 * No plaintext table exists.
 */
class ProtectedTrackingStore(private val persistence: ProtectedEnterprisePersistence) : EnterpriseTrackingStore {
    /** The index is business evidence: an unreadable or foreign one is unavailable and never replaced. */
    private fun book(snapshot: VaultSnapshot, attempt: EnterpriseQueueAttempt): TrackingBook? {
        val stored = snapshot.entries[KEY] ?: return null
        return try {
            TrackingBook.decode(stored.bytes).requireBinding(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
        } catch (_: Exception) { throw EnterprisePersistenceUnavailable() }
    }

    private fun trips(snapshot: VaultSnapshot, attempt: EnterpriseQueueAttempt): TripBook? {
        val stored = snapshot.entries[TRIP_KEY] ?: return null
        return try {
            TripBook.decode(stored.bytes).requireBinding(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
        } catch (_: Exception) { throw EnterprisePersistenceUnavailable() }
    }

    /** EMP-GPS-003: the sealed buffer record; unreadable or foreign is unavailable and never replaced. */
    private fun buffers(snapshot: VaultSnapshot, attempt: EnterpriseQueueAttempt): TrackingBufferBook? {
        val stored = snapshot.entries[BUFFER_KEY] ?: return null
        return try {
            TrackingBufferBook.decode(stored.bytes).requireBinding(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
        } catch (_: Exception) { throw EnterprisePersistenceUnavailable() }
    }

    override suspend fun read(attempt: EnterpriseQueueAttempt): EnterpriseTrackingSnapshot = persistence.queueAccess(attempt) { snapshot ->
        val book = book(snapshot, attempt)
        val trips = trips(snapshot, attempt)
        val buffers = buffers(snapshot, attempt)
        val ids = (book?.sessions?.flatMap { it.operationIds } ?: emptyList()) + (trips?.trips?.flatMap { it.operationIds } ?: emptyList()) +
            (buffers?.sessions?.flatMap { it.batchOperationIds } ?: emptyList())
        val entries = ids.distinct().associateWith { id ->
            val entry = snapshot.entries["QUEUE:$id"] ?: throw EnterprisePersistenceUnavailable()
            ProtectedQueueCodec.decode(entry.bytes, snapshot, entry.id)
        }
        VaultQueueChange(EnterpriseTrackingSnapshot(book, trips, entries, buffers))
    }

    override suspend fun updateBuffer(attempt: EnterpriseQueueAttempt, sessionId: String, change: (SessionBuffer?) -> SessionBuffer?): SessionBuffer? =
        persistence.queueAccess(attempt) { snapshot ->
            val member = attempt.enrollment.target.membershipId
            val enrollment = attempt.enrollment.enrollmentId
            // A buffer exists only for a session of this partition's book (never rebound, B-BR-EMP-503).
            if (book(snapshot, attempt)?.sessionById(sessionId) == null) return@queueAccess VaultQueueChange<SessionBuffer?>(null)
            val current = buffers(snapshot, attempt) ?: TrackingBufferBook.empty(member, enrollment)
            val next = change(current.session(sessionId)) ?: return@queueAccess VaultQueueChange(current.session(sessionId))
            require(next.sessionId == sessionId)
            val bytes = current.with(next).canonicalBytes
            snapshot.entries[BUFFER_KEY] = VaultEntry(VaultKind.RECORD, EnterpriseTrackingProducers.BUFFER_RECORD_ID, "", bytes)
            VaultQueueChange(next, changed = true)
        }

    private suspend fun update(attempt: EnterpriseQueueAttempt, change: (TrackingBook?) -> TrackingBook?): Boolean =
        persistence.queueAccess(attempt) { snapshot ->
            val next = change(book(snapshot, attempt)) ?: return@queueAccess VaultQueueChange(false)
            next.requireBinding(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
            val bytes = next.canonicalBytes
            if (snapshot.entries[KEY]?.bytes?.contentEquals(bytes) == true) VaultQueueChange(true)
            else {
                snapshot.entries[KEY] = VaultEntry(VaultKind.RECORD, EnterpriseTrackingProducers.BOOK_RECORD_ID, "", bytes)
                VaultQueueChange(true, changed = true)
            }
        }

    override suspend fun replacePolicy(attempt: EnterpriseQueueAttempt, read: TrackingPolicyRead) {
        update(attempt) { (it ?: TrackingBook.empty(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)).withPolicy(read) }
    }

    override suspend fun checkpoint(attempt: EnterpriseQueueAttempt, sessionId: String, checkpoint: TrackingCheckpoint): Boolean =
        update(attempt) { current -> current?.takeIf { it.sessionById(sessionId) != null }?.withCheckpoint(sessionId, checkpoint) }

    override fun toString(): String = "ProtectedTrackingStore(redacted)"

    private companion object {
        const val KEY = "RECORD:" + EnterpriseTrackingProducers.BOOK_RECORD_ID
        const val TRIP_KEY = "RECORD:" + EnterpriseTripProducers.BOOK_RECORD_ID
        const val BUFFER_KEY = "RECORD:" + EnterpriseTrackingProducers.BUFFER_RECORD_ID
    }
}
