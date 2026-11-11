package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.domain.enterprise.*
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

data class EnterpriseLocalIdentity(val account: String, val origin: String, val family: String) {
    init {
        EnterprisePartition.requireId(account); EnterprisePartition.requireId(family)
        require(EnterprisePartition.canonicalOrigin(origin) == origin)
    }
    fun owns(p: EnterprisePartition): Boolean = account == p.ownerAccount && origin == p.origin
    override fun toString(): String = "EnterpriseLocalIdentity(redacted)"
}

data class EnterpriseContactGrant(
    val partition: EnterprisePartition,
    val family: String,
    val serverMillis: Long,
    val validityMillis: Long = EnterpriseOfflineLease.DEFAULT_VALIDITY_MILLIS,
    val managedUnlockSeconds: Int? = null,
) {
    override fun toString(): String = "EnterpriseContactGrant(redacted)"
}

/** Implement only behind verified current server account/device/membership proof, never device input. */
fun interface EnterpriseContactAuthority {
    suspend fun confirm(partition: EnterprisePartition): EnterpriseContactGrant
}

object UnavailableEnterpriseContact : EnterpriseContactAuthority {
    override suspend fun confirm(partition: EnterprisePartition): EnterpriseContactGrant = throw EnterpriseLocalDenied()
}

/** Ready for native validation; the public persistence binding remains closed until acceptance. */
class ProtectedEnterprisePersistence(
    private val files: EnterpriseVaultFiles,
    private val keys: EnterpriseVaultKeys,
    private val fence: EnterpriseSessionFence,
    private val erasure: EnterpriseVaultErasure,
    private val identity: () -> EnterpriseLocalIdentity?,
    private val clock: () -> EnterpriseDeviceClock,
    private val authority: EnterpriseContactAuthority,
) : EnterprisePersistence {
    private fun owner(p: EnterprisePartition): EnterpriseLocalIdentity {
        val current = try { identity() } catch (_: Exception) { null }
        return current?.takeIf { it.owns(p) } ?: throw EnterpriseLocalDenied()
    }

    private fun snapshot(p: EnterprisePartition): Pair<VaultSnapshot, EnterpriseVaultKey> {
        val token = EnterpriseVaultCodec.token(p)
        if (erasure.deniedLocked(token)) throw EnterpriseLocalDenied()
        val bytes = files.read(token) ?: throw EnterpriseLocalDenied()
        val key = keys.existing(EnterpriseVaultCodec.alias(token)) ?: throw EnterpriseLocalDenied()
        val value = EnterpriseVaultCodec.open(bytes, p, key.secret)
        if (key.managedUnlockSeconds != value.lease.managedUnlockSeconds) throw EnterpriseLocalDenied()
        return value to key
    }

    /** Call before selecting a context. Late contact cannot unlock a changed/logged-out session. */
    suspend fun confirm(p: EnterprisePartition, acknowledgeErasedWork: Boolean = false) {
        val before = fence.access { Triple(owner(p), fence.epoch, fence.backgroundEpoch) }
        val grant = try { authority.confirm(p) }
        catch (cancelled: CancellationException) { throw cancelled }
        catch (denied: EnterpriseLocalDenied) { throw denied }
        catch (_: Exception) { throw EnterprisePersistenceUnavailable() }
        guarded {
            fence.access {
                if (before.first != owner(p) || before.second != fence.epoch ||
                    grant.partition != p || grant.family != before.first.family
                ) throw EnterpriseLocalDenied()
                files.locked(EnterpriseVaultCodec.accountTag(p.ownerAccount, p.origin)) {
                    val token = EnterpriseVaultCodec.token(p)
                    val denied = erasure.deniedLocked(token)
                    val missingKey = files.read(token) != null && keys.existing(EnterpriseVaultCodec.alias(token)) == null
                    val fresh = denied || missingKey
                    if (fresh && (!acknowledgeErasedWork || !erasure.prepareFreshLocked(token))) throw EnterpriseLocalDenied()
                    val now = clock()
                    val old = files.read(token)
                    val pair = if (old != null) snapshot(p) else null
                    val prior = pair?.first
                    if (prior != null) {
                        val lease = prior.lease
                        if (grant.serverMillis < lease.serverContactMillis) throw EnterpriseLocalDenied()
                        if (EnterpriseOfflineProtection.evaluate(lease, now) == EnterpriseLeaseDecision.ERASE ||
                            grant.serverMillis - lease.serverContactMillis >= lease.validityMillis
                        ) {
                            erasure.eraseLocked(token)
                            throw EnterpriseLocalDenied()
                        }
                        if (lease.managedUnlockSeconds != grant.managedUnlockSeconds) throw EnterpriseLocalDenied()
                    }
                    val lease = EnterpriseOfflineLease(grant.family, grant.serverMillis, now, grant.validityMillis, grant.managedUnlockSeconds)
                    val key = pair?.second ?: keys.existing(EnterpriseVaultCodec.alias(token))
                        ?: keys.create(EnterpriseVaultCodec.alias(token), grant.managedUnlockSeconds)
                    if (key.managedUnlockSeconds != grant.managedUnlockSeconds) throw EnterpriseLocalDenied()
                    val value = prior ?: VaultSnapshot(p, lease)
                    value.lease = lease
                    files.replace(token, EnterpriseVaultCodec.seal(value, key.secret))
                    if (before.first != owner(p) || before.third != fence.backgroundEpoch) throw EnterpriseLocalDenied()
                    if (fresh) erasure.permitFreshLocked(token)
                    fence.lock()
                }
            }
        }
    }

    private suspend fun <T> guarded(work: () -> T): T = withContext(Dispatchers.IO) {
        try { work() } catch (cancelled: CancellationException) { throw cancelled }
        catch (denied: EnterpriseLocalDenied) { throw denied }
        catch (reused: EnterpriseOperationReused) { throw reused }
        catch (_: Exception) { throw EnterprisePersistenceUnavailable() }
    }

    private suspend fun <T> access(p: EnterprisePartition, write: Boolean, work: (VaultSnapshot) -> T): T {
        val epoch = fence.epoch
        return guarded {
            fence.access {
                if (epoch != fence.epoch) throw EnterpriseLocalDenied()
                val before = owner(p)
                files.locked(EnterpriseVaultCodec.accountTag(p.ownerAccount, p.origin)) {
                    val (s, key) = snapshot(p)
                    if (s.lease.sessionFamily != before.family) throw EnterpriseLocalDenied()
                    when (EnterpriseOfflineProtection.evaluate(s.lease, clock())) {
                        EnterpriseLeaseDecision.ERASE -> {
                            erasure.eraseLocked(EnterpriseVaultCodec.token(p))
                            throw EnterpriseLocalDenied()
                        }
                        EnterpriseLeaseDecision.RECONFIRM -> { fence.lock(); throw EnterpriseLocalDenied() }
                        EnterpriseLeaseDecision.ALLOW -> Unit
                    }
                    val result = work(s)
                    if (write) files.replace(EnterpriseVaultCodec.token(p), EnterpriseVaultCodec.seal(s, key.secret))
                    if (before != owner(p) || epoch != fence.epoch) throw EnterpriseLocalDenied()
                    result
                }
            }
        }
    }

    override suspend fun record(partition: EnterprisePartition, id: String): EnterpriseLocalRecord? {
        EnterprisePartition.requireId(id)
        return access(partition, false) { s ->
            s.entries["RECORD:" + id]?.let { EnterpriseLocalRecord(partition, id, it.bytes) }
        }
    }

    override suspend fun put(record: EnterpriseLocalRecord) = access(record.partition, true) { s ->
        s.entries["RECORD:" + record.id] = VaultEntry(VaultKind.RECORD, record.id, "", record.envelope)
    }

    override suspend fun enqueue(operation: EnterprisePendingOperation): Boolean = access(operation.partition, true) { s ->
        val key = "OPERATION:" + operation.id
        val prior = s.entries[key]
        if (prior != null) {
            if (prior.target != operation.targetId || !prior.bytes.contentEquals(operation.envelope)) throw EnterpriseOperationReused()
            false
        } else {
            s.entries[key] = VaultEntry(VaultKind.OPERATION, operation.id, operation.targetId, operation.envelope)
            true
        }
    }

    override suspend fun pending(partition: EnterprisePartition, limit: Int): List<EnterprisePendingOperation> {
        require(limit in 1..50) { "Invalid enterprise page size" }
        return access(partition, false) { s ->
            s.entries.values.filter { it.kind == VaultKind.OPERATION }.take(limit).map {
                EnterprisePendingOperation(partition, it.id, it.target, it.bytes)
            }
        }
    }

    /** Attachments and derived caches never materialize as plaintext files or paths. */
    suspend fun putPrivateFile(p: EnterprisePartition, id: String, bytes: ByteArray, cache: Boolean = false) {
        EnterprisePartition.requireId(id)
        val copy = bytes.copyOf()
        try {
            access(p, true) { s ->
                val kind = if (cache) VaultKind.CACHE else VaultKind.ATTACHMENT
                s.entries[kind.name + ":" + id] = VaultEntry(kind, id, "", copy)
            }
        } finally { copy.fill(0) }
    }

    suspend fun privateFile(p: EnterprisePartition, id: String, cache: Boolean = false): ByteArray? {
        EnterprisePartition.requireId(id)
        return access(p, false) { s ->
            s.entries[(if (cache) "CACHE:" else "ATTACHMENT:") + id]?.bytes?.copyOf()
        }
    }
}
