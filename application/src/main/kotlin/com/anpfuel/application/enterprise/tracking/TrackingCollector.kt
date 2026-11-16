package com.anpfuel.application.enterprise.tracking

import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueSource
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.tracking.LocationPermission
import com.anpfuel.domain.enterprise.tracking.TrackingCause
import com.anpfuel.domain.enterprise.tracking.TrackingObservation
import com.anpfuel.domain.enterprise.tracking.TrackingObservationOutcome
import java.time.Instant
import kotlinx.coroutines.channels.Channel
import java.util.concurrent.atomic.AtomicLong
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/*
 * EMP-GPS-002 collection runtime (B-BR-EMP-496/498/499, EDR-323): the one location listener of the organization
 * tracking foreground service, bound to the partition and session that the visible trip screen granted. The platform is
 * a port so the Android service, the host tests and the device acceptance drive the same rules.
 */

/** One platform location report; it never leaves the collector in this task and prints redacted. */
class TrackingFixSample(val latitude: Double, val longitude: Double, val accuracyMetres: Float?, val speedMetresPerSecond: Float?,
    val bearingDegrees: Float?, val wallMillis: Long, val elapsedMillis: Long, val mock: Boolean) {
    override fun toString(): String = "EnterpriseTrackingFixSample(redacted)"
}

/** What the collector needs from the platform; the Android adapter owns the one LocationManager subscription. */
interface TrackingPlatform {
    /** The location permission granted now (fine → PRECISE, coarse only → APPROXIMATE). */
    fun permission(): LocationPermission
    /** The ongoing notification can be seen (permission granted and channel not blocked). */
    fun notificationVisible(): Boolean
    fun bootId(): String
    /** Registers the single listener for [precision]; false when the platform refused. Called only while detached. */
    fun attach(precision: LocationPermission, sink: (TrackingFixSample) -> Unit): Boolean
    /** Removes this collector's listener only. */
    fun detach()
    /** EMP-GPS-003: the device wall clock and monotonic clock, read together at a report (EDR-325). */
    fun wallMillis(): Long
    fun elapsedMillis(): Long
    /** Below 15 % battery and not charging (BR-GPS-019). */
    fun batteryLow(): Boolean
    /** The provider of [precision] is enabled (a disabled one opens PROVIDER_DISABLED at once). */
    fun providerEnabled(precision: LocationPermission): Boolean
}

/**
 * The one-time start grant (B-BR-EMP-496): armed by the visible trip screen for one partition and session, valid for
 * [TTL_MILLIS] of elapsed time, consumed by the first start; anything else is not a grant.
 */
class TrackingStartGate(private val elapsedMillis: () -> Long) {
    private class Grant(val partition: EnterprisePartition, val sessionId: String, val armedAt: Long)
    private var grant: Grant? = null
    @Synchronized fun arm(partition: EnterprisePartition, sessionId: String) {
        EnterprisePartition.requireId(sessionId)
        grant = Grant(partition, sessionId, elapsedMillis())
    }
    @Synchronized fun consume(partition: EnterprisePartition, sessionId: String): Boolean {
        val current = grant ?: return false
        grant = null
        val age = elapsedMillis() - current.armedAt
        return current.partition == partition && current.sessionId == sessionId && age in 0..TTL_MILLIS
    }
    override fun toString(): String = "EnterpriseTrackingStartGate(redacted)"
    companion object { const val TTL_MILLIS = 30_000L }
}

enum class TrackingBeginResult { STARTED, ALREADY_RUNNING, NO_GRANT, BUSY, NOT_RECORDING, PERMISSION_MISSING, NOTIFICATION_HIDDEN, PLATFORM_REFUSED, UNAVAILABLE }

/** Counts and state only, never coordinates (B-BR-EMP-500). */
data class TrackingRuntimeState(val active: Boolean = false, val sessionId: String? = null, val precision: LocationPermission? = null,
    val accepted: Long = 0, val stoppedBy: TrackingCause? = null, val recorded: Long = 0, val economy: Boolean = false) {
    override fun toString(): String = "TrackingRuntimeState(redacted)"
}

class TrackingCollector(
    private val service: EnterpriseTrackingService,
    private val source: EnterpriseQueueSource,
    private val gate: TrackingStartGate,
    private val platform: TrackingPlatform,
    /** EMP-GPS-003: one delivery attempt of the partition after batches were sealed (the ordinary queue). */
    private val deliver: suspend (EnterpriseQueueAccountLease, EnterprisePartition) -> Unit = { _, _ -> },
) {
    /** A report with the clocks read when it arrived (EDR-325). */
    private class Received(val owner: Any, val sample: TrackingFixSample, val wall: Long, val elapsed: Long)
    private val inbox = Channel<Received>(capacity = INBOX)
    private class Binding(val partition: EnterprisePartition, val sessionId: String, val tripId: String, val identity: com.anpfuel.application.enterprise.EnterpriseLocalIdentity,
        val membershipId: String, val enrollmentId: String, val precision: LocationPermission)
    private val lock = Mutex()
    @Volatile private var binding: Binding? = null
    private val accepted = AtomicLong()
    private val mutable = MutableStateFlow(TrackingRuntimeState())
    val state: StateFlow<TrackingRuntimeState> = mutable.asStateFlow()
    /** The session this process is collecting for, if any (the recovery check of B-BR-EMP-499). */
    val runningSessionId: String? get() = binding?.sessionId

    private suspend fun lease(): EnterpriseQueueAccountLease = source.account()

    /** Consumes the screen's grant, then attaches the one listener for a recording session of an open trip. */
    suspend fun begin(partition: EnterprisePartition, sessionId: String): TrackingBeginResult = lock.withLock {
        val granted = gate.consume(partition, sessionId)
        binding?.let { current ->
            return if (current.partition == partition && current.sessionId == sessionId) TrackingBeginResult.ALREADY_RUNNING else TrackingBeginResult.BUSY
        }
        if (!granted) return TrackingBeginResult.NO_GRANT
        val account = try { lease() } catch (cancelled: CancellationException) { throw cancelled } catch (_: Exception) { return TrackingBeginResult.UNAVAILABLE }
        val attempt = try { source.attempt(account, partition) } catch (cancelled: CancellationException) { throw cancelled } catch (_: Exception) { return TrackingBeginResult.UNAVAILABLE }
        val collecting = try { service.collecting(account, partition, sessionId) } catch (cancelled: CancellationException) { throw cancelled }
            catch (_: Exception) { return TrackingBeginResult.UNAVAILABLE } ?: return TrackingBeginResult.NOT_RECORDING
        val permission = platform.permission()
        if (permission == LocationPermission.NONE) return TrackingBeginResult.PERMISSION_MISSING
        if (!platform.notificationVisible()) return TrackingBeginResult.NOTIFICATION_HIDDEN
        // Precision may only fall between a start and the next one (EDR-275): never collect finer than the session's.
        val precision = if (collecting.precision == LocationPermission.APPROXIMATE) LocationPermission.APPROXIMATE else permission
        val next = Binding(partition, sessionId, collecting.tripId, account.identity, attempt.enrollment.target.membershipId,
            attempt.enrollment.enrollmentId, precision)
        binding = next
        if (!platform.attach(precision) { onFix(next, it) }) {
            binding = null
            return TrackingBeginResult.PLATFORM_REFUSED
        }
        mutable.value = TrackingRuntimeState(active = true, sessionId = sessionId, precision = precision, accepted = accepted.get())
        try { service.checkpoint(account, partition, sessionId, platform.bootId()) } catch (cancelled: CancellationException) { throw cancelled } catch (_: Exception) { }
        TrackingBeginResult.STARTED
    }

    private fun onFix(owner: Binding, sample: TrackingFixSample) {
        // A report that arrives after a stop or for a replaced binding is dropped, never counted (BR-GPS-013).
        if (binding !== owner) return
        if (!sample.latitude.isFinite() || !sample.longitude.isFinite()) return
        mutable.value = mutable.value.copy(accepted = accepted.incrementAndGet())
        // Durable recording happens off the callback, in arrival order; a full inbox is counted, never blocking the platform.
        inbox.trySend(Received(owner, sample, platform.wallMillis(), platform.elapsedMillis()))
    }

    /** EMP-GPS-003: records every report queued so far through the sampler and the sealed buffer (B-BR-EMP-501..503). */
    suspend fun drain(): Int {
        var processed = 0
        while (true) {
            val next = inbox.tryReceive().getOrNull() ?: return processed
            process(next); processed++
        }
    }

    /** The service's consumer loop: records reports as they arrive until the collector is closed. */
    suspend fun run() { for (next in inbox) process(next) }

    private suspend fun process(received: Received) = lock.withLock {
        val current = binding ?: return@withLock
        if (current !== received.owner && current.sessionId != (received.owner as Binding).sessionId) return@withLock
        val s = received.sample
        val economy = platform.batteryLow()
        val outcome = try {
            service.record(lease(), current.partition, current.sessionId, TrackingObservation(s.latitude, s.longitude, s.accuracyMetres,
                s.speedMetresPerSecond, s.bearingDegrees, s.mock, s.elapsedMillis), economy, received.elapsed, Instant.ofEpochMilli(received.wall),
                platform.bootId())
        } catch (cancelled: CancellationException) { throw cancelled } catch (_: Exception) { TrackingObservationOutcome.IGNORED }
        val recorded = mutable.value.recorded + if (outcome == TrackingObservationOutcome.RECORDED) 1 else 0
        mutable.value = mutable.value.copy(recorded = recorded, economy = economy)
    }

    private fun detachLocked(stoppedBy: TrackingCause?) {
        if (binding == null) return
        binding = null
        platform.detach()
        mutable.value = TrackingRuntimeState(active = false, accepted = accepted.get(), stoppedBy = stoppedBy)
    }

    /** Stops listening without writing anything (the trip screen already wrote the transition, or the owner changed). */
    suspend fun release(stoppedBy: TrackingCause? = null): Unit = lock.withLock { detachLocked(stoppedBy) }

    /** "Parar rastreamento" from the notification: interrupts the session (STOPPED_BY_DRIVER) and releases the listener. */
    suspend fun stopByDriver(): Boolean { drain(); return stopLocked() }

    private suspend fun stopLocked(): Boolean = lock.withLock {
        val current = binding ?: return false
        try {
            val account = lease()
            service.collecting(account, current.partition, current.sessionId)?.let { service.stop(account, current.partition, it.tripId) }
        } catch (cancelled: CancellationException) { throw cancelled } catch (_: Exception) {
            // The checkpoint stays; the next opening of the trip screen records the interruption (B-BR-EMP-499).
        }
        detachLocked(TrackingCause.STOPPED_BY_DRIVER)
        true
    }

    /**
     * The 30-second watchdog (BR-GPS-015): owner, trip and session first, then permission, precision and notification;
     * a failed check records the cause it requires and releases the listener. True while still collecting.
     */
    suspend fun tick(): Boolean { drain(); return tickLocked() }

    private suspend fun tickLocked(): Boolean = lock.withLock {
        val current = binding ?: return false
        val account = try { lease() } catch (cancelled: CancellationException) { throw cancelled } catch (_: Exception) { null }
        val same = account != null && account.identity == current.identity && account.identity.owns(current.partition) && try {
            val attempt = source.attempt(account, current.partition)
            attempt.enrollment.target.membershipId == current.membershipId && attempt.enrollment.enrollmentId == current.enrollmentId
        } catch (cancelled: CancellationException) { throw cancelled } catch (_: Exception) { false }
        if (!same) {
            // Another account, context or membership: stop at once; the session is never rebound (BR-GPS-014).
            if (account != null) runCatching { service.interrupt(account, current.partition, current.sessionId, TrackingCause.OWNER_CHANGED) }
            detachLocked(TrackingCause.OWNER_CHANGED)
            return false
        }
        account!!
        try {
            val collecting = service.collecting(account, current.partition, current.sessionId)
            if (collecting == null) {
                // The trip finished or was refused, or the session left RECORDING elsewhere: follow, then stop listening.
                service.follow(account, current.partition, current.tripId)
                detachLocked(null)
                return false
            }
            val permission = platform.permission()
            val cause = when {
                permission == LocationPermission.NONE -> TrackingCause.PERMISSION_REVOKED
                !platform.notificationVisible() -> TrackingCause.NOTIFICATION_BLOCKED
                else -> null
            }
            if (cause != null) {
                service.interrupt(account, current.partition, current.sessionId, cause)
                detachLocked(cause)
                return false
            }
            if (permission == LocationPermission.APPROXIMATE && collecting.precision == LocationPermission.PRECISE) {
                service.reduce(account, current.partition, current.sessionId)
            }
            if (permission == LocationPermission.APPROXIMATE && current.precision == LocationPermission.PRECISE) {
                // One listener at a time: the precise subscription is replaced by the approximate one.
                platform.detach()
                val reduced = Binding(current.partition, current.sessionId, current.tripId, current.identity, current.membershipId,
                    current.enrollmentId, LocationPermission.APPROXIMATE)
                binding = reduced
                if (!platform.attach(LocationPermission.APPROXIMATE) { onFix(reduced, it) }) { binding = null; return false }
                mutable.value = mutable.value.copy(precision = LocationPermission.APPROXIMATE)
            }
            service.checkpoint(account, current.partition, current.sessionId, platform.bootId())
            // EMP-GPS-003: gaps are data (B-BR-EMP-505); due fixes leave in a batch and the queue is asked to deliver.
            val precision = binding?.precision ?: current.precision
            service.checkGap(account, current.partition, current.sessionId, platform.providerEnabled(precision),
                Instant.ofEpochMilli(platform.wallMillis()), platform.bootId())
            if (service.seal(account, current.partition, current.sessionId, force = false) > 0) {
                try { deliver(account, current.partition) } catch (cancelled: CancellationException) { throw cancelled } catch (_: Exception) { }
            }
            true
        } catch (cancelled: CancellationException) { throw cancelled } catch (_: EnterpriseLocalDenied) {
            detachLocked(TrackingCause.OWNER_CHANGED)
            false
        } catch (_: Exception) {
            // A storage outage keeps the listener; the next tick checks again (the checkpoint is not advanced).
            true
        }
    }

    override fun toString(): String = "EnterpriseTrackingCollector(redacted)"

    private companion object { const val INBOX = 256 }
}

/**
 * The process's tracking runtime (EDR-323): the visible-screen start gate and the one running collector, so the trip
 * screen can tell a live collector from a dead process (B-BR-EMP-499). Process memory only; never persisted.
 */
class TrackingRuntime(val gate: TrackingStartGate) {
    @Volatile var collector: TrackingCollector? = null
    fun running(sessionId: String): Boolean = collector?.runningSessionId == sessionId
    override fun toString(): String = "EnterpriseTrackingRuntime(redacted)"
}
