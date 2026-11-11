package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.domain.enterprise.*
import java.io.IOException
import javax.crypto.AEADBadTagException
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

    /** What one partition's stored ciphertext currently allows. */
    private sealed interface Stored {
        data object Absent : Stored
        /** Ciphertext without its key: restored elsewhere or partially erased. */
        data object Lost : Stored
        /** Invalidated or different key, or corrupted/truncated ciphertext: never readable again. */
        data object Unreadable : Stored
        class Open(val snapshot: VaultSnapshot, val key: EnterpriseVaultKey) : Stored
    }

    private fun keyFailure(failure: Exception): Exception = when (keys.classify(failure)) {
        EnterpriseKeyUse.UNLOCK_REQUIRED -> EnterpriseUnlockRequired()
        EnterpriseKeyUse.INVALIDATED -> EnterpriseVaultUnrecoverable()
        EnterpriseKeyUse.OTHER -> failure
    }

    private fun stored(p: EnterprisePartition, token: String): Stored {
        val bytes = try { files.read(token) } catch (_: EnterpriseVaultCorrupted) { return Stored.Unreadable }
            ?: return Stored.Absent
        val key = keys.existing(EnterpriseVaultCodec.alias(token)) ?: return Stored.Lost
        val value = try {
            EnterpriseVaultCodec.open(bytes, p, key.secret)
        } catch (failure: Exception) {
            val mapped = keyFailure(failure)
            // An authenticated decrypt that fails, or a malformed envelope, cannot succeed on retry.
            if (mapped is EnterpriseVaultUnrecoverable || failure is AEADBadTagException ||
                failure is IllegalArgumentException || failure is IOException
            ) return Stored.Unreadable
            throw mapped
        }
        // A recorded policy that differs from the key's means the key was replaced outside this adapter.
        if (key.managedUnlockSeconds != value.lease.managedUnlockSeconds) return Stored.Unreadable
        return Stored.Open(value, key)
    }

    private fun snapshot(p: EnterprisePartition): Pair<VaultSnapshot, EnterpriseVaultKey> {
        val token = EnterpriseVaultCodec.token(p)
        if (erasure.deniedLocked(token)) throw EnterpriseLocalDenied()
        return when (val current = stored(p, token)) {
            Stored.Absent, Stored.Lost -> throw EnterpriseLocalDenied()
            Stored.Unreadable -> throw EnterpriseVaultUnrecoverable()
            is Stored.Open -> current.snapshot to current.key
        }
    }

    private fun seal(value: VaultSnapshot, key: EnterpriseVaultKey): ByteArray =
        try { EnterpriseVaultCodec.seal(value, key.secret) } catch (failure: Exception) { throw keyFailure(failure) }

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
                    val alias = EnterpriseVaultCodec.alias(token)
                    val denied = erasure.deniedLocked(token)
                    // Unlock-required propagates here: it is never a reason to erase.
                    val current = if (denied) null else stored(p, token)
                    val now = clock()
                    val prior = current as? Stored.Open
                    if (prior != null) {
                        val lease = prior.snapshot.lease
                        if (grant.serverMillis < lease.serverContactMillis) throw EnterpriseLocalDenied()
                        if (EnterpriseOfflineProtection.evaluate(lease, now) == EnterpriseLeaseDecision.ERASE ||
                            grant.serverMillis - lease.serverContactMillis >= lease.validityMillis
                        ) {
                            erasure.eraseLocked(token)
                            throw EnterpriseLocalDenied()
                        }
                    }
                    // No re-keying migration exists: a changed managed-unlock policy, like an
                    // unreadable vault, continues only from an acknowledged empty start.
                    val unreadable = current == Stored.Unreadable ||
                        (prior != null && prior.snapshot.lease.managedUnlockSeconds != grant.managedUnlockSeconds)
                    val fresh = denied || current == Stored.Lost || unreadable
                    if (fresh && !acknowledgeErasedWork) {
                        throw if (unreadable) EnterpriseVaultUnrecoverable() else EnterpriseLocalDenied()
                    }
                    if (fresh && !erasure.prepareFreshLocked(token)) throw EnterpriseLocalDenied()
                    val kept = prior.takeUnless { fresh }
                    val lease = EnterpriseOfflineLease(grant.family, grant.serverMillis, now, grant.validityMillis, grant.managedUnlockSeconds)
                    val key = kept?.key ?: keys.existing(alias) ?: keys.create(alias, grant.managedUnlockSeconds)
                    if (key.managedUnlockSeconds != grant.managedUnlockSeconds) throw EnterpriseLocalDenied()
                    val value = kept?.snapshot ?: VaultSnapshot(p, lease)
                    value.lease = lease
                    files.replace(token, seal(value, key))
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
        catch (unlock: EnterpriseUnlockRequired) { throw unlock }
        catch (lost: EnterpriseVaultUnrecoverable) { throw lost }
        catch (_: Exception) { throw EnterprisePersistenceUnavailable() }
        // A failed bounded snapshot allocation leaves no partial state: writes are atomic
        // replacements and every lock is released on the way out, so it is an outage.
        catch (_: OutOfMemoryError) { throw EnterprisePersistenceUnavailable() }
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
                    if (write) files.replace(EnterpriseVaultCodec.token(p), seal(s, key))
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
