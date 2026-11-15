package com.anpfuel.data.enterprise.fleet

import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.fleet.EnterpriseReferenceCacheStore
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.data.enterprise.ProtectedEnterprisePersistence
import com.anpfuel.data.enterprise.VaultEntry
import com.anpfuel.data.enterprise.VaultKind
import com.anpfuel.data.enterprise.VaultQueueChange
import com.anpfuel.domain.enterprise.fleet.ReferenceCache

/**
 * The FLEET-013 "local tables" (EDR-224): one sealed CACHE entry of the exact partition's
 * protected generation holding the versioned reference cache. No plaintext table exists.
 * Every read and write goes through the exact attempt's custody, enrollment and lease checks.
 */
class ProtectedReferenceCacheStore(private val persistence: ProtectedEnterprisePersistence) : EnterpriseReferenceCacheStore {
    private fun bound(entry: VaultEntry?, attempt: EnterpriseQueueAttempt): ReferenceCache? {
        if (entry == null || entry.kind != VaultKind.CACHE) return null
        // An unknown version or shape is a stale derived copy, never authority: read as absent.
        val cache = try { ReferenceCache.decode(entry.bytes) } catch (_: IllegalArgumentException) { return null }
        return cache.takeIf {
            it.membershipId == attempt.enrollment.target.membershipId && it.enrollmentId == attempt.enrollment.enrollmentId
        }
    }

    override suspend fun read(attempt: EnterpriseQueueAttempt): ReferenceCache? =
        persistence.queueAccess(attempt) { snapshot -> VaultQueueChange(bound(snapshot.entries[KEY], attempt)) }

    override suspend fun replace(attempt: EnterpriseQueueAttempt, expected: ReferenceCache?, next: ReferenceCache): Boolean {
        if (next.membershipId != attempt.enrollment.target.membershipId || next.enrollmentId != attempt.enrollment.enrollmentId)
            throw EnterpriseLocalDenied()
        val bytes = next.canonicalBytes
        return persistence.queueAccess(attempt) { snapshot ->
            val stored = bound(snapshot.entries[KEY], attempt)
            if (if (expected == null) stored != null else !expected.sameState(stored)) VaultQueueChange(false)
            else {
                snapshot.entries[KEY] = VaultEntry(VaultKind.CACHE, ENTRY_ID, "", bytes)
                VaultQueueChange(true, changed = true)
            }
        }
    }

    override fun toString(): String = "ProtectedReferenceCacheStore(redacted)"

    companion object {
        /** Fixed opaque identifier of the one reference cache entry per partition. */
        const val ENTRY_ID = "f13ca7e5-0000-4000-8000-000000000001"
        private const val KEY = "CACHE:$ENTRY_ID"
    }
}
