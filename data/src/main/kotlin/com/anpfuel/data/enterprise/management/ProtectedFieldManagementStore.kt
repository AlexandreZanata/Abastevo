package com.anpfuel.data.enterprise.management

import com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable
import com.anpfuel.application.enterprise.management.EnterpriseFieldManagementStore
import com.anpfuel.application.enterprise.management.EnterpriseManagementProducers
import com.anpfuel.application.enterprise.management.FieldManagementBook
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.data.enterprise.ProtectedEnterprisePersistence
import com.anpfuel.data.enterprise.VaultEntry
import com.anpfuel.data.enterprise.VaultKind
import com.anpfuel.data.enterprise.VaultQueueChange

/**
 * Protected local storage for F6 compliance notices and field maintenance
 * requests inside the encrypted enterprise vault (no Room/plaintext storage).
 */
class ProtectedFieldManagementStore(
    private val persistence: ProtectedEnterprisePersistence,
) : EnterpriseFieldManagementStore {
    override suspend fun read(attempt: EnterpriseQueueAttempt): FieldManagementBook? =
        persistence.queueAccess(attempt) { snapshot ->
            val stored = snapshot.entries[BOOK_KEY] ?: return@queueAccess VaultQueueChange(null)
            val book = try {
                FieldManagementBook.decode(stored.bytes)
                    .requireBinding(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
            } catch (_: Exception) {
                throw EnterprisePersistenceUnavailable()
            }
            val manifest = snapshot.entries[MANIFEST_KEY]
            if (manifest != null && !manifest.bytes.contentEquals(book.maintenanceManifestBytes)) {
                throw EnterprisePersistenceUnavailable()
            }
            VaultQueueChange(book)
        }

    override suspend fun save(attempt: EnterpriseQueueAttempt, book: FieldManagementBook) {
        persistence.queueAccess(attempt) { snapshot ->
            val bound = try {
                book.requireBinding(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
            } catch (_: Exception) {
                throw EnterprisePersistenceUnavailable()
            }
            val bookBytes = bound.canonicalBytes
            val manifestBytes = bound.maintenanceManifestBytes
            val unchanged = snapshot.entries[BOOK_KEY]?.bytes?.contentEquals(bookBytes) == true &&
                snapshot.entries[MANIFEST_KEY]?.bytes?.contentEquals(manifestBytes) == true
            if (unchanged) {
                VaultQueueChange(Unit)
            } else {
                snapshot.entries[BOOK_KEY] = VaultEntry(
                    kind = VaultKind.RECORD,
                    id = EnterpriseManagementProducers.COMPLIANCE_BOOK_RECORD_ID,
                    target = "",
                    bytes = bookBytes,
                )
                snapshot.entries[MANIFEST_KEY] = VaultEntry(
                    kind = VaultKind.RECORD,
                    id = EnterpriseManagementProducers.MAINTENANCE_REQUESTS_RECORD_ID,
                    target = "",
                    bytes = manifestBytes,
                )
                VaultQueueChange(Unit, changed = true)
            }
        }
    }

    override fun toString(): String = "ProtectedFieldManagementStore(redacted)"

    private companion object {
        const val BOOK_KEY = "RECORD:" + EnterpriseManagementProducers.COMPLIANCE_BOOK_RECORD_ID
        const val MANIFEST_KEY = "RECORD:" + EnterpriseManagementProducers.MAINTENANCE_REQUESTS_RECORD_ID
    }
}
