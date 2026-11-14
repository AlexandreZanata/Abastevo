package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.domain.enterprise.*
import java.io.IOException
import javax.crypto.AEADBadTagException
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/** Exact source-bound custody, authenticated generations and independent live-context fencing. */
class ProtectedEnterprisePersistence(
    private val files: EnterpriseVaultFiles,
    private val keys: EnterpriseVaultKeys,
    private val fence: EnterpriseSessionFence,
    private val erasure: EnterpriseVaultErasure,
    private val identity: () -> EnterpriseLocalIdentity?,
    private val clock: () -> EnterpriseDeviceClock,
    private val authority: EnterpriseContactAuthority,
) : EnterprisePersistence, EnterpriseContextProtection {
    /** The source obtains exact retained enrollment facts without opening a managed payload key. */
    suspend fun protectedEnrollment(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseEnrollmentBinding = guarded {
        fence.access {
            checkedQueueAccount(account, partition)
            files.locked(EnterpriseVaultCodec.accountTag(partition.ownerAccount, partition.origin)) {
                checkedQueueAccount(account, partition)
                val descriptor = queueCandidate(account, EnterpriseVaultCodec.token(partition)).descriptor ?: throw EnterpriseLocalDenied()
                if (descriptor.partition != partition) throw EnterpriseLocalDenied()
                checkedQueueAccount(account, partition)
                descriptor.enrollment()
            }
        }
    }

    /** A new invocation starts with null; only this current account's opaque candidates are inspected. */
    suspend fun discover(account: EnterpriseQueueAccountLease, afterOpaque: String? = null,
        limit: Int = 50): EnterpriseQueueDiscoveryPage {
        require(limit in 1..50)
        return guarded {
            fence.access {
                checkedQueueAccount(account)
                val tag = EnterpriseVaultCodec.accountTag(account.identity.account, account.identity.origin)
                if (afterOpaque != null && (!Regex("[0-9a-f]{64}-[0-9a-f]{64}").matches(afterOpaque) ||
                    !afterOpaque.startsWith("$tag-"))) throw EnterpriseLocalDenied()
                // A personal-only account has no protected footprint. Discovering it creates nothing,
                // not even the account lock file, so it can never later look like lost data (EDR-122).
                // A concurrent first save is simply found by a later invocation.
                if (!hasFootprint(tag)) {
                    checkedQueueAccount(account)
                    return@access EnterpriseQueueDiscoveryPage(emptyList(), null, 0)
                }
                files.locked(tag) {
                    checkedQueueAccount(account)
                    val tokens = sortedSetOf<String>()
                    tokens += files.tokens(tag)
                    for (prefix in listOf(EnterpriseVaultGenerationNames.PREFIX + tag + "-",
                        EnterpriseVaultGenerationNames.PREFIX + "denied_" + tag + "-",
                        EnterpriseVaultGenerationNames.PREFIX + "lock_" + tag + "-")) {
                        tokens += keys.aliases(prefix).mapNotNull(EnterpriseVaultGenerationNames::token)
                    }
                    check(tokens.size <= AtomicEnterpriseVaultFiles.MAX_DISCOVERY)
                    val candidates = tokens.filter { it.startsWith("$tag-") && (afterOpaque == null || it > afterOpaque) }.take(limit)
                    val bindings = mutableListOf<EnterpriseEnrollmentBinding>()
                    var unavailable = 0
                    var contactRequired = 0
                    for (token in candidates) {
                        checkedQueueAccount(account)
                        try {
                            val candidate = queueCandidate(account, token)
                            candidate.descriptor?.let { bindings += it.enrollment() }
                            if (candidate.contactRequired) contactRequired++
                        } catch (denied: EnterpriseLocalDenied) { throw denied }
                        catch (_: Exception) { unavailable++ }
                    }
                    checkedQueueAccount(account)
                    val last = candidates.lastOrNull()
                    EnterpriseQueueDiscoveryPage(bindings, last?.takeIf { token -> tokens.any { it > token } }, unavailable, contactRequired)
                }
            }
        }
    }

    /** Caller predicates are an additional fence, never a replacement for the actual store's source. */
    private fun checkedQueueAccount(account: EnterpriseQueueAccountLease, partition: EnterprisePartition? = null) {
        account.assertCurrent()
        val current = try { identity() } catch (_: Exception) { null }
        if (current != account.identity || fence.epoch != account.epoch || fence.backgroundEpoch != account.background ||
            (partition != null && !account.identity.owns(partition))) throw EnterpriseLocalDenied()
    }

    private fun VaultMaintenance.enrollment() = EnterpriseEnrollmentBinding(
        EnterpriseDeviceTarget(partition, lease.membershipId), lease.enrollmentId, lease.enrollmentRevision)

    /**
     * Opaque scope check only: real material or a durable denial/requirement, never a bare lock
     * file. Uses only the exact scoped prefixes the actual Android key provider accepts.
     */
    private fun hasFootprint(tag: String): Boolean = files.present(tag) ||
        keys.existing(EnterpriseVaultGenerationNames.denied(tag)) != null || listOf(
            EnterpriseVaultGenerationNames.PREFIX + tag + "-",
            EnterpriseVaultGenerationNames.PREFIX + "denied_" + tag + "-",
            EnterpriseVaultGenerationNames.PREFIX + "lock_" + tag + "-",
            EnterpriseVaultCodec.alias(tag + "-"),
        ).any { keys.aliases(it).isNotEmpty() }

    /** A null descriptor with [contactRequired] waits for the person's explicit reconfirmation. */
    private class QueueCandidate(val descriptor: VaultMaintenance? = null, val contactRequired: Boolean = false) {
        override fun toString() = "QueueCandidate(redacted)"
    }

    private fun queueCandidate(account: EnterpriseQueueAccountLease, token: String): QueueCandidate {
        if (erasure.deniedLocked(token)) return ABSENT
        val descriptor = generations.descriptor(token) ?: return ABSENT
        if (!account.identity.owns(descriptor.partition) || descriptor.lease.time.sessionFamily != account.identity.family) return ABSENT
        val decision = EnterpriseOfflineProtection.evaluate(descriptor.lease.time, clock())
        // Proven privacy expiry still wins. Otherwise an existing durable requirement is checked
        // before any RECONFIRM side effect: it is attention, not a failure, and fences nothing.
        if (decision == EnterpriseLeaseDecision.ERASE) { erasure.eraseLocked(token); return ABSENT }
        if (keys.existing(EnterpriseVaultGenerationNames.retained(token)) != null) return CONTACT_REQUIRED
        if (decision == EnterpriseLeaseDecision.RECONFIRM) { erasure.requireContactLocked(token); return CONTACT_REQUIRED }
        return QueueCandidate(descriptor)
    }

    internal suspend fun <T> queueAccess(attempt: EnterpriseQueueAttempt,
        work: (VaultSnapshot) -> VaultQueueChange<T>): T {
        attempt.assertCurrent()
        val result = access(attempt.partition, true, commitWhen = { it.changed },
            afterCommit = { checkedQueueAttempt(attempt, it) }) { snapshot ->
            checkedQueueAttempt(attempt, snapshot)
            val change = work(snapshot)
            checkedQueueAttempt(attempt, snapshot)
            change
        }
        checkedQueueAccount(attempt.account, attempt.partition)
        return result.value
    }

    /** Status cursors observe the same authenticated head as their borrowed snapshot. */
    internal suspend fun <T> queueStatusAccess(attempt: EnterpriseQueueAttempt,
        work: (VaultSnapshot, Long) -> VaultQueueChange<T>): T = queueAccess(attempt) { snapshot ->
        val generation = generations.head(EnterpriseVaultCodec.token(snapshot.partition)) ?: throw EnterpriseVaultUnrecoverable()
        work(snapshot, generation)
    }

    private fun checkedQueueAttempt(attempt: EnterpriseQueueAttempt, snapshot: VaultSnapshot) {
        attempt.assertCurrent()
        checkedQueueAccount(attempt.account, snapshot.partition)
        val bound = snapshot.bound ?: throw EnterpriseVaultUnrecoverable()
        val expected = EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(snapshot.partition, bound.membershipId),
            bound.enrollmentId, bound.enrollmentRevision)
        if (snapshot.partition != attempt.partition || expected != attempt.enrollment ||
            snapshot.lease.sessionFamily != attempt.account.identity.family) throw EnterpriseLocalDenied()
        when (EnterpriseOfflineProtection.evaluate(snapshot.lease, clock())) {
            EnterpriseLeaseDecision.ERASE -> { erasure.eraseLocked(EnterpriseVaultCodec.token(snapshot.partition)); throw EnterpriseLocalDenied() }
            EnterpriseLeaseDecision.RECONFIRM -> { erasure.requireContactLocked(EnterpriseVaultCodec.token(snapshot.partition)); throw EnterpriseLocalDenied() }
            EnterpriseLeaseDecision.ALLOW -> Unit
        }
    }

    private fun owner(p: EnterprisePartition): EnterpriseLocalIdentity {
        val current = try { identity() } catch (_: Exception) { null }
        return current?.takeIf { it.owns(p) } ?: throw EnterpriseLocalDenied()
    }

    private val generations = EnterpriseVaultGenerations(files, keys)

    private fun checkedOwner(target: EnterpriseDeviceTarget, binding: EnterpriseAccessBinding, contact: EnterpriseFenceOwner): EnterpriseLocalIdentity {
        val current = owner(target.partition)
        if (current.account != binding.accountId || current.origin != binding.origin || current.family != binding.familyId ||
            !fence.ownerIsCurrent(contact, binding.epoch)) throw EnterpriseLocalDenied()
        return current
    }

    /** The request-start observation conservatively includes all signing/bootstrap/network latency. */
    override suspend fun confirm(target: EnterpriseDeviceTarget, binding: EnterpriseAccessBinding, contact: EnterpriseFenceOwner,
        acknowledgeErasedWork: Boolean): EnterpriseFenceHandoff {
        val p = target.partition
        val token = EnterpriseVaultCodec.token(p)
        val before = fence.access { checkedOwner(target, binding, contact) to clock() }
        val prior = guarded {
            fence.access {
                checkedOwner(target, binding, contact)
                files.locked(EnterpriseVaultCodec.accountTag(p.ownerAccount, p.origin)) {
                    if (erasure.deniedLocked(token) && !acknowledgeErasedWork) throw EnterpriseVaultUnrecoverable()
                    try { generations.descriptor(token) }
                    catch (lost: EnterpriseVaultUnrecoverable) { if (!acknowledgeErasedWork) throw lost else null }
                }
            }
        }
        val previousEnrollment = prior?.lease?.let {
            EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(p, it.membershipId), it.enrollmentId, it.enrollmentRevision)
        }
        val changedMember = previousEnrollment != null && previousEnrollment.target != target
        if (changedMember && !acknowledgeErasedWork) throw EnterpriseVaultUnrecoverable()
        val reply = try { authority.confirm(EnterpriseContactRequest(binding, target, previousEnrollment.takeUnless { changedMember })) }
        catch (cancelled: CancellationException) { throw cancelled }
        catch (refused: EnterpriseProtectionRefused) { throw refused }
        catch (denied: EnterpriseLocalDenied) { throw denied }
        catch (_: Exception) { throw EnterprisePersistenceUnavailable() }
        return guarded {
            fence.access {
                if (checkedOwner(target, binding, contact) != before.first) throw EnterpriseLocalDenied()
                files.locked(EnterpriseVaultCodec.accountTag(p.ownerAccount, p.origin)) {
                    val enrollment = when (reply) {
                        is EnterpriseContactReply.Active -> reply.grant.enrollment
                        is EnterpriseContactReply.Erase -> reply.enrollment
                        is EnterpriseContactReply.Lock -> reply.enrollment
                    }
                    val family = when (reply) {
                        is EnterpriseContactReply.Active -> reply.grant.family
                        is EnterpriseContactReply.Erase -> reply.family
                        is EnterpriseContactReply.Lock -> reply.family
                    }
                    if (enrollment.target != target || family != before.first.family) throw EnterpriseLocalDenied()
                    if (!changedMember && previousEnrollment != null &&
                        (enrollment.enrollmentId != previousEnrollment.enrollmentId || enrollment.revision < previousEnrollment.revision)) {
                        throw EnterpriseVaultUnrecoverable()
                    }
                    // Re-read the trusted head after network I/O; another writer cannot be silently overwritten.
                    val currentDescriptor = try { generations.descriptor(token) } catch (lost: EnterpriseVaultUnrecoverable) {
                        if (!acknowledgeErasedWork) throw lost else null
                    }
                    if (currentDescriptor != prior) throw EnterpriseLocalDenied()
                    when (reply) {
                        is EnterpriseContactReply.Erase -> { erasure.eraseLocked(token); throw EnterpriseVaultUnrecoverable() }
                        is EnterpriseContactReply.Lock -> {
                            erasure.requireContactLocked(token); throw EnterpriseLocalDenied()
                        }
                        is EnterpriseContactReply.Active -> {
                            val grant = reply.grant
                            val oldLease = prior?.lease?.time
                            if (oldLease != null && grant.serverMillis < oldLease.serverContactMillis) throw EnterpriseLocalDenied()
                            val completedClock = clock()
                            val expired = oldLease != null && (EnterpriseOfflineProtection.evaluate(oldLease, completedClock) == EnterpriseLeaseDecision.ERASE ||
                                grant.serverMillis - oldLease.serverContactMillis >= oldLease.validityMillis)
                            if (expired && !acknowledgeErasedWork) { erasure.eraseLocked(token); throw EnterpriseVaultUnrecoverable() }
                            val changed = changedMember || (previousEnrollment != null && previousEnrollment.enrollmentId != enrollment.enrollmentId) ||
                                (oldLease != null && oldLease.managedUnlockSeconds != grant.managedUnlockSeconds)
                            var unreadable = false
                            val priorSnapshot = if (erasure.deniedLocked(token) || changed || expired) null else try { generations.load(p) }
                                catch (lost: EnterpriseVaultUnrecoverable) { unreadable = true; null }
                            val fresh = erasure.deniedLocked(token) || changed || expired || unreadable || (prior != null && priorSnapshot == null)
                            if (fresh && !acknowledgeErasedWork) throw EnterpriseVaultUnrecoverable()
                            if (fresh && !erasure.prepareFreshLocked(token)) throw EnterpriseLocalDenied()
                            val lease = EnterpriseOfflineLease(grant.family, grant.serverMillis, before.second, grant.validityMillis, grant.managedUnlockSeconds)
                            if (EnterpriseOfflineProtection.evaluate(lease, completedClock) != EnterpriseLeaseDecision.ALLOW) throw EnterpriseLocalDenied()
                            val snapshot = if (fresh) VaultSnapshot(p, lease) else priorSnapshot ?: VaultSnapshot(p, lease)
                            snapshot.lease = lease
                            snapshot.bound = EnterpriseBoundOfflineLease(enrollment.target.membershipId, enrollment.enrollmentId, enrollment.revision, lease)
                            generations.commit(snapshot)
                            if (checkedOwner(target, binding, contact) != before.first) throw EnterpriseLocalDenied()
                            if (fresh) erasure.permitFreshLocked(token)
                            keys.delete(EnterpriseVaultGenerationNames.retained(token))
                            fence.commitHandoff(contact, binding.epoch)
                        }
                    }
                }
            }
        }
    }

    private suspend fun <T> guarded(work: () -> T): T = withContext(Dispatchers.IO) {
        try { work() } catch (cancelled: CancellationException) { throw cancelled }
        catch (refused: EnterpriseProtectionRefused) { throw refused }
        catch (denied: EnterpriseLocalDenied) { throw denied }
        catch (reused: EnterpriseOperationReused) { throw reused }
        catch (unlock: EnterpriseUnlockRequired) { throw unlock }
        catch (lost: EnterpriseVaultUnrecoverable) { throw lost }
        catch (_: Exception) { throw EnterprisePersistenceUnavailable() }
        // A failed bounded snapshot allocation leaves no partial state: writes are atomic
        // replacements and every lock is released on the way out, so it is an outage.
        catch (_: OutOfMemoryError) { throw EnterprisePersistenceUnavailable() }
    }

    private suspend fun <T> access(p: EnterprisePartition, write: Boolean,
        commitWhen: (T) -> Boolean = { write }, afterCommit: (VaultSnapshot) -> Unit = { }, work: (VaultSnapshot) -> T): T {
        val epoch = fence.epoch
        return guarded {
            fence.access {
                if (epoch != fence.epoch) throw EnterpriseLocalDenied()
                val before = owner(p)
                files.locked(EnterpriseVaultCodec.accountTag(p.ownerAccount, p.origin)) {
                    val token = EnterpriseVaultCodec.token(p)
                    if (erasure.deniedLocked(token)) throw EnterpriseLocalDenied()
                    val descriptor = generations.descriptor(token) ?: throw EnterpriseLocalDenied()
                    if (descriptor.lease.time.sessionFamily != before.family) throw EnterpriseLocalDenied()
                    when (EnterpriseOfflineProtection.evaluate(descriptor.lease.time, clock())) {
                        EnterpriseLeaseDecision.ERASE -> { erasure.eraseLocked(token); throw EnterpriseLocalDenied() }
                        EnterpriseLeaseDecision.RECONFIRM -> { erasure.requireContactLocked(token); throw EnterpriseLocalDenied() }
                        EnterpriseLeaseDecision.ALLOW -> Unit
                    }
                    if (keys.existing(EnterpriseVaultGenerationNames.retained(token)) != null) throw EnterpriseLocalDenied()
                    val s = generations.load(p) ?: throw EnterpriseLocalDenied()
                    if (s.bound == null) throw EnterpriseVaultUnrecoverable()
                    val result = work(s)
                    if (write && commitWhen(result)) generations.commit(s)
                    afterCommit(s)
                    if (before != owner(p) || epoch != fence.epoch || erasure.deniedLocked(token)) throw EnterpriseLocalDenied()
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

    private companion object {
        val ABSENT = QueueCandidate()
        val CONTACT_REQUIRED = QueueCandidate(contactRequired = true)
    }
}

internal class VaultQueueChange<T>(val value: T, val changed: Boolean = false)
