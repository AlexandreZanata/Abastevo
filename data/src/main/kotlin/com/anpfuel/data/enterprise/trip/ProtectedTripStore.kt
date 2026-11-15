package com.anpfuel.data.enterprise.trip

import com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.trip.EnterpriseTripProducers
import com.anpfuel.application.enterprise.trip.EnterpriseTripSnapshot
import com.anpfuel.application.enterprise.trip.EnterpriseTripStore
import com.anpfuel.data.enterprise.ProtectedEnterprisePersistence
import com.anpfuel.data.enterprise.VaultQueueChange
import com.anpfuel.data.enterprise.protectedqueue.ProtectedQueueCodec
import com.anpfuel.domain.enterprise.trip.TripBook

/**
 * The EMP-TRIP-005 "local trip storage" (EDR-242): the sealed trip-book RECORD of the exact partition's protected
 * generation and the retained queue entries it names, read in one snapshot through the attempt's custody,
 * enrollment and lease checks. No plaintext table exists.
 */
class ProtectedTripStore(private val persistence: ProtectedEnterprisePersistence) : EnterpriseTripStore {
    override suspend fun read(attempt: EnterpriseQueueAttempt): EnterpriseTripSnapshot = persistence.queueAccess(attempt) { snapshot ->
        val stored = snapshot.entries["RECORD:" + EnterpriseTripProducers.BOOK_RECORD_ID]
            ?: return@queueAccess VaultQueueChange(EnterpriseTripSnapshot(null, emptyMap()))
        // The index is business evidence: an unreadable or foreign one is unavailable and never replaced (EDR-242).
        val book = try {
            TripBook.decode(stored.bytes).requireBinding(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
        } catch (_: Exception) { throw EnterprisePersistenceUnavailable() }
        val entries = book.trips.flatMap { it.operationIds }.associateWith { id ->
            val entry = snapshot.entries["QUEUE:$id"] ?: throw EnterprisePersistenceUnavailable()
            ProtectedQueueCodec.decode(entry.bytes, snapshot, entry.id)
        }
        VaultQueueChange(EnterpriseTripSnapshot(book, entries))
    }
    override fun toString(): String = "ProtectedTripStore(redacted)"
}
