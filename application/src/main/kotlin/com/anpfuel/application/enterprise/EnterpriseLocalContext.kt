package com.anpfuel.application.enterprise

import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterprisePendingOperation
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.util.concurrent.atomic.AtomicLong

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
/** The managed device unlock must happen before this partition can be read or written. */
class EnterpriseUnlockRequired : IllegalStateException("Enterprise device unlock required")
/** Stored work can never be opened again; only acknowledged erasure plus fresh contact recovers the partition. */
class EnterpriseVaultUnrecoverable : IllegalStateException("Enterprise protected storage unrecoverable")

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
    private val ownership = Any()
    private var current: EnterprisePartition? = null
    private val generation = AtomicLong()
    class Owner internal constructor()
    class Lease internal constructor(internal val owner: Owner, internal val generation: Long)
    private var owner: Owner? = null

    /** Invalidates old handles immediately, including I/O in progress, without changing creation ownership. */
    fun invalidate() = synchronized(ownership) { owner = null; generation.incrementAndGet(); Unit }

    /** An explicit new selection attempt replaces local ownership before any suspension or server contact. */
    fun acquire(requestOwner: Owner): Lease = synchronized(ownership) {
        owner = requestOwner
        Lease(requestOwner, generation.incrementAndGet())
    }

    /** An outgoing flow can invalidate only its own selection, never a replacement flow's handles. */
    fun invalidate(requestOwner: Owner): Lease? = synchronized(ownership) {
        if (owner !== requestOwner) null else Lease(requestOwner, generation.incrementAndGet())
    }

    private fun owns(lease: Lease): Boolean = owner === lease.owner && generation.get() == lease.generation

    suspend fun select(partition: EnterprisePartition): BoundStore = mutex.withLock {
        synchronized(ownership) {
            // Legacy feature handles inherit the active flow owner, while the scope remains explicit.
            current = null
            val token = generation.incrementAndGet()
            if (!matchesIdentity(partition)) throw EnterpriseLocalDenied()
            current = partition
            BoundStore(partition, token, accessEpoch())
        }
    }

    suspend fun select(lease: Lease, partition: EnterprisePartition): BoundStore = mutex.withLock {
        synchronized(ownership) {
            // Check after the mutex wait, before touching another attempt's selection or generation.
            if (!owns(lease)) throw EnterpriseLocalDenied()
            current = null
            val token = generation.incrementAndGet() // Consume the immutable lease; it cannot revive old handles.
            if (!matchesIdentity(partition)) throw EnterpriseLocalDenied()
            current = partition
            BoundStore(partition, token, accessEpoch())
        }
    }

    suspend fun lock() = mutex.withLock {
        synchronized(ownership) { current = null; owner = null; generation.incrementAndGet(); Unit }
    }

    suspend fun lock(lease: Lease): Boolean = mutex.withLock {
        synchronized(ownership) {
            if (!owns(lease)) false else { current = null; true }
        }
    }

    private fun matchesIdentity(partition: EnterprisePartition): Boolean = try {
        ownerAccount() == partition.ownerAccount &&
            EnterprisePartition.canonicalOrigin(serverOrigin()) == partition.origin
    } catch (_: Exception) { false }

    private suspend fun <T> scoped(partition: EnterprisePartition, token: Long, epoch: Long, block: suspend () -> T): T = mutex.withLock {
        if (current != partition || generation.get() != token || accessEpoch() != epoch) throw EnterpriseLocalDenied()
        if (!matchesIdentity(partition) || accessEpoch() != epoch) {
            current = null
            generation.compareAndSet(token, token + 1)
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
        } catch (unlock: EnterpriseUnlockRequired) {
            throw unlock
        } catch (lost: EnterpriseVaultUnrecoverable) {
            throw lost
        } catch (_: IllegalArgumentException) {
            throw IllegalArgumentException("Invalid enterprise local request")
        } catch (_: Exception) {
            // SQL/storage errors may include private row contents or paths.
            throw EnterprisePersistenceUnavailable()
        }
        // Account/session changes may happen independently of this local selection.
        if (!matchesIdentity(partition) || accessEpoch() != epoch || generation.get() != token) {
            current = null
            // A newer attempt may have reserved a lease while this I/O held the mutex.
            // Do not advance or revoke its generation merely because the old result is withheld.
            generation.compareAndSet(token, token + 1)
            throw EnterpriseLocalDenied()
        }
        result
    }

    inner class BoundStore internal constructor(val partition: EnterprisePartition, private val token: Long, private val epoch: Long) {
        fun isCurrent(): Boolean = generation.get() == token && accessEpoch() == epoch && matchesIdentity(partition)
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
