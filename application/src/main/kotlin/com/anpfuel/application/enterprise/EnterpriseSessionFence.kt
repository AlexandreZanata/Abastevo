package com.anpfuel.application.enterprise

import java.util.concurrent.locks.ReentrantLock
import java.util.concurrent.atomic.AtomicLong
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.concurrent.withLock
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/** Reserved by one exact local selection attempt; it cannot follow replacement ownership. */
class EnterpriseFenceOwner internal constructor(
    internal val local: EnterpriseLocalContext,
    internal val lease: EnterpriseLocalContext.Lease,
    internal val binding: EnterpriseAccessBinding,
    internal val background: Long,
    internal val fence: EnterpriseSessionFence,
    internal val currentBinding: () -> EnterpriseAccessBinding?,
) {
    @Volatile internal var handoff: EnterpriseFenceHandoff? = null
    override fun toString(): String = "EnterpriseFenceOwner(redacted)"
}

class EnterpriseFenceHandoff internal constructor(
    internal val owner: EnterpriseFenceOwner,
    val previousEpoch: Long,
    val nextEpoch: Long,
    internal val lease: EnterpriseLocalContext.Lease,
) {
    internal val consumed = AtomicBoolean()
    override fun toString(): String = "EnterpriseFenceHandoff(redacted)"
}

/** Shared with session mutation, local I/O and backgrounding; contains no credentials. */
class EnterpriseSessionFence {
    private val mutex = ReentrantLock()
    private val generation = AtomicLong()
    private val background = AtomicLong()
    private val pendingPublication = ThreadLocal<Long?>()
    private val observedEpoch = MutableStateFlow(0L)
    val epochs: StateFlow<Long> = observedEpoch.asStateFlow()
    val epoch: Long get() = generation.get()
    val backgroundEpoch: Long get() = background.get()
    private fun publish(next: Long) {
        if (mutex.isHeldByCurrentThread) pendingPublication.set(maxOf(pendingPublication.get() ?: next, next))
        else observedEpoch.update { maxOf(it, next, generation.get()) }
    }
    private fun advance() {
        publish(generation.incrementAndGet())
    }
    fun lock() { access { advance() } }
    // Main-thread lifecycle invalidation must not wait for filesystem I/O.
    fun markBackground() {
        background.incrementAndGet()
        advance()
    }
    fun <T> access(work: () -> T): T = try {
        mutex.withLock(work)
    } finally {
        if (!mutex.isHeldByCurrentThread) {
            val pending = pendingPublication.get()
            pendingPublication.remove()
            if (pending != null) publish(pending)
        }
    }

    fun ownerIsCurrent(owner: EnterpriseFenceOwner, expectedEpoch: Long): Boolean =
        owner.fence === this && expectedEpoch == owner.binding.epoch && epoch == expectedEpoch &&
            backgroundEpoch == owner.background && owner.handoff == null && owner.local.retainsContact(owner)

    /** Called only after the matching durable protected generation has committed. */
    fun commitHandoff(owner: EnterpriseFenceOwner, expectedEpoch: Long): EnterpriseFenceHandoff = access {
        if (expectedEpoch == Long.MAX_VALUE || !ownerIsCurrent(owner, expectedEpoch)) throw EnterpriseLocalDenied()
        val handoff = owner.local.publishContact(owner) { successor ->
            val next = expectedEpoch + 1
            val receipt = EnterpriseFenceHandoff(owner, expectedEpoch, next, successor)
            // Register before publication so an epoch observer can recognize
            // only this exact in-flight selection. It has not consumed it yet.
            owner.handoff = receipt
            if (backgroundEpoch != owner.background || !generation.compareAndSet(expectedEpoch, next)) {
                owner.handoff = null
                throw EnterpriseLocalDenied()
            }
            receipt
        }
        // StateFlow may resume an Unconfined observer synchronously. Never
        // publish while holding local ownership or an enclosing fence access.
        publish(handoff.nextEpoch)
        if (backgroundEpoch != owner.background || epoch != handoff.nextEpoch) throw EnterpriseLocalDenied()
        handoff
    }

    fun handoffIsCurrent(owner: EnterpriseFenceOwner, handoff: EnterpriseFenceHandoff, binding: EnterpriseAccessBinding?): Boolean =
        owner.fence === this && handoff.owner === owner && owner.handoff === handoff &&
            handoff.previousEpoch == owner.binding.epoch && handoff.nextEpoch == handoff.previousEpoch + 1 &&
            epoch == handoff.nextEpoch && backgroundEpoch == owner.background &&
            binding == owner.binding.copy(epoch = handoff.nextEpoch) && owner.local.retainsHandoff(owner, handoff, binding)
}
