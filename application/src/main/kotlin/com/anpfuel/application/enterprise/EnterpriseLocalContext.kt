package com.anpfuel.application.enterprise

import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterprisePendingOperation
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/** All ports require a creation scope. There is no unscoped enumeration or dispatcher. */
interface EnterprisePersistence {
    suspend fun record(partition: EnterprisePartition, id: String): EnterpriseLocalRecord?
    suspend fun put(record: EnterpriseLocalRecord)
    suspend fun enqueue(operation: EnterprisePendingOperation): Boolean
    suspend fun pending(partition: EnterprisePartition, limit: Int): List<EnterprisePendingOperation>
}

class EnterpriseLocalDenied : IllegalStateException("Enterprise local context unavailable")
class EnterprisePersistenceUnavailable : IllegalStateException("Enterprise protected storage unavailable")
class EnterpriseOperationReused : IllegalStateException("Enterprise operation reused")

/** Production binding stays closed until EMP-PRIV-003 supplies protected persistence. */
object UnavailableEnterprisePersistence : EnterprisePersistence {
    override suspend fun record(partition: EnterprisePartition, id: String): EnterpriseLocalRecord? = throw EnterprisePersistenceUnavailable()
    override suspend fun put(record: EnterpriseLocalRecord): Unit = throw EnterprisePersistenceUnavailable()
    override suspend fun enqueue(operation: EnterprisePendingOperation): Boolean = throw EnterprisePersistenceUnavailable()
    override suspend fun pending(partition: EnterprisePartition, limit: Int): List<EnterprisePendingOperation> = throw EnterprisePersistenceUnavailable()
}

/**
 * An in-memory selection is never persisted or treated as membership authority.
 * The owner/origin ports are current account identity and shared API configuration.
 * Selection/lock serializes with local calls; stale handles never follow a new scope.
 */
class EnterpriseLocalContext(
    private val ownerAccount: () -> String?,
    private val serverOrigin: () -> String,
    private val persistence: EnterprisePersistence,
    private val accessEpoch: () -> Long = { 0L },
) {
    private val mutex = Mutex()
    private var current: EnterprisePartition? = null
    private var generation = 0L

    suspend fun select(partition: EnterprisePartition): BoundStore = mutex.withLock {
        // Even a refused switch invalidates the previously selected handle.
        current = null
        generation++
        if (!matchesIdentity(partition)) throw EnterpriseLocalDenied()
        current = partition
        BoundStore(partition, generation, accessEpoch())
    }

    suspend fun lock() = mutex.withLock {
        current = null
        generation++
    }

    private fun matchesIdentity(partition: EnterprisePartition): Boolean = try {
        ownerAccount() == partition.ownerAccount &&
            EnterprisePartition.canonicalOrigin(serverOrigin()) == partition.origin
    } catch (_: Exception) { false }

    private suspend fun <T> scoped(partition: EnterprisePartition, token: Long, epoch: Long, block: suspend () -> T): T = mutex.withLock {
        if (current != partition || generation != token || accessEpoch() != epoch) throw EnterpriseLocalDenied()
        if (!matchesIdentity(partition) || accessEpoch() != epoch) {
            current = null
            generation++
            throw EnterpriseLocalDenied()
        }
        val result = try {
            block()
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (denied: EnterpriseLocalDenied) {
            throw denied
        } catch (reused: EnterpriseOperationReused) {
            throw reused
        } catch (_: IllegalArgumentException) {
            throw IllegalArgumentException("Invalid enterprise local request")
        } catch (_: Exception) {
            // SQL/storage errors may include private row contents or paths.
            throw EnterprisePersistenceUnavailable()
        }
        // Account/session changes may happen independently of this local selection.
        if (!matchesIdentity(partition) || accessEpoch() != epoch) {
            current = null
            generation++
            throw EnterpriseLocalDenied()
        }
        result
    }

    inner class BoundStore internal constructor(val partition: EnterprisePartition, private val token: Long, private val epoch: Long) {
        suspend fun record(id: String): EnterpriseLocalRecord? = scoped(partition, token, epoch) {
            val row = persistence.record(partition, EnterprisePartition.requireId(id))
            if (row != null && (row.partition != partition || row.id != id)) throw EnterpriseLocalDenied()
            row
        }

        suspend fun put(record: EnterpriseLocalRecord) = scoped(partition, token, epoch) {
            if (record.partition != partition) throw EnterpriseLocalDenied()
            persistence.put(record)
        }

        suspend fun enqueue(operation: EnterprisePendingOperation): Boolean = scoped(partition, token, epoch) {
            if (operation.partition != partition) throw EnterpriseLocalDenied()
            persistence.enqueue(operation)
        }

        suspend fun pending(limit: Int = 50): List<EnterprisePendingOperation> = scoped(partition, token, epoch) {
            // Existing bounded operation-batch precedent; no feature retry policy.
            require(limit in 1..50) { "Invalid enterprise page size" }
            persistence.pending(partition, limit).also { rows ->
                if (rows.size > limit || rows.any { it.partition != partition }) throw EnterpriseLocalDenied()
            }
        }

        override fun toString(): String = "EnterpriseBoundStore(redacted)"
    }
}
