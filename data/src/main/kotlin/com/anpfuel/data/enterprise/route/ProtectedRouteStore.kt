package com.anpfuel.data.enterprise.route

import com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.route.EnterpriseRouteProducers
import com.anpfuel.application.enterprise.route.EnterpriseRouteStore
import com.anpfuel.data.enterprise.ProtectedEnterprisePersistence
import com.anpfuel.data.enterprise.VaultEntry
import com.anpfuel.data.enterprise.VaultKind
import com.anpfuel.data.enterprise.VaultQueueChange
import com.anpfuel.domain.enterprise.route.EnterpriseRouteBook
import com.anpfuel.domain.enterprise.route.EnterpriseRoutePlan

/**
 * Protected local route storage bound to the partition's membership and
 * enrollment inside the encrypted enterprise vault (no Room/plaintext tables).
 */
class ProtectedRouteStore(private val persistence: ProtectedEnterprisePersistence) : EnterpriseRouteStore {
    override suspend fun read(attempt: EnterpriseQueueAttempt): EnterpriseRouteBook? =
        persistence.queueAccess(attempt) { snapshot ->
            val stored = snapshot.entries[KEY] ?: return@queueAccess VaultQueueChange(null)
            val book = try {
                EnterpriseRouteBook.decode(stored.bytes)
                    .requireBinding(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
            } catch (_: Exception) {
                throw EnterprisePersistenceUnavailable()
            }
            VaultQueueChange(book)
        }

    override suspend fun savePlan(attempt: EnterpriseQueueAttempt, plan: EnterpriseRoutePlan) {
        persistence.queueAccess(attempt) { snapshot ->
            val member = attempt.enrollment.target.membershipId
            val enrollment = attempt.enrollment.enrollmentId
            val existing = snapshot.entries[KEY]?.let { stored ->
                try {
                    EnterpriseRouteBook.decode(stored.bytes).requireBinding(member, enrollment)
                } catch (_: Exception) {
                    throw EnterprisePersistenceUnavailable()
                }
            } ?: EnterpriseRouteBook.empty(member, enrollment)
            val updated = existing.withPlan(plan)
            val bytes = updated.canonicalBytes
            if (snapshot.entries[KEY]?.bytes?.contentEquals(bytes) == true) {
                VaultQueueChange(Unit)
            } else {
                snapshot.entries[KEY] = VaultEntry(VaultKind.RECORD, EnterpriseRouteProducers.BOOK_RECORD_ID, "", bytes)
                VaultQueueChange(Unit, changed = true)
            }
        }
    }

    override fun toString(): String = "ProtectedRouteStore(redacted)"

    private companion object {
        const val KEY = "RECORD:" + EnterpriseRouteProducers.BOOK_RECORD_ID
    }
}
