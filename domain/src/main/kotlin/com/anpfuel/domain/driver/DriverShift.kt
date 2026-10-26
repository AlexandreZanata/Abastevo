package com.anpfuel.domain.driver

import java.math.BigInteger
import java.security.MessageDigest
import java.time.ZoneId

/** Half-open intervals in seconds from one declared UTC origin. */
data class DriverTimeSpan(val from: Long, val to: Long) { init { require(from >= 0 && to > from) } }
object DriverOnlineTime {
    fun unionSeconds(spans: List<DriverTimeSpan>): Long {
        require(spans.size <= DRIVER_MAX_ITEMS)
        if (spans.isEmpty()) return 0
        val sorted = spans.sortedBy { it.from }
        var start = sorted.first().from; var end = sorted.first().to; var total = 0L
        sorted.drop(1).forEach { if (it.from > end) { total = Math.addExact(total, end - start); start = it.from; end = it.to } else end = maxOf(end, it.to) }
        return Math.addExact(total, end - start)
    }
}
data class DriverClock(val bootId: String, val elapsedMillis: Long, val utcMillis: Long) {
    init { require(bootId.matches(Regex("[A-Za-z0-9._-]{1,64}")) && elapsedMillis >= 0) }
}
data class DriverClockPolicy(val maximumCheckpointGapMillis: Long, val wallToleranceMillis: Long) {
    init { require(maximumCheckpointGapMillis > 0 && wallToleranceMillis >= 0) }
}
enum class DriverShiftState { READY, RUNNING, PAUSED, CLOSED, RECOVERY_REQUIRED }
enum class DriverShiftAction { START, PAUSE, RESUME, END, CHECKPOINT, RECOVER, REVIEW_GAP }
data class DriverShiftCommand(val id: String, val action: DriverShiftAction, val expectedVersion: Long, val clock: DriverClock,
                              val gapId: String? = null, val reviewedWorkMillis: Long? = null) {
    init { require(id.matches(Regex("[A-Za-z0-9._-]{1,64}")) && expectedVersion >= 0 && (reviewedWorkMillis == null || reviewedWorkMillis >= 0)) }
    internal fun fingerprint(): String = MessageDigest.getInstance("SHA-256").digest(
        "$action|$expectedVersion|${clock.bootId}|${clock.elapsedMillis}|${clock.utcMillis}|$gapId|$reviewedWorkMillis".toByteArray(Charsets.UTF_8),
    ).joinToString("") { "%02x".format(it) }
}
data class DriverShiftGap(val id: String, val reason: String, val previous: DriverClock, val next: DriverClock, val reviewedWorkMillis: Long? = null)
data class DriverShiftReceipt(val id: String, val fingerprint: String, val acceptedVersion: Long)
data class DriverShiftTransition(val shift: DriverShift, val receipt: DriverShiftReceipt, val replayed: Boolean)
data class DriverShift(
    val id: String, val owner: String, val origin: String, val vehicle: String, val timezone: String,
    val state: DriverShiftState = DriverShiftState.READY, val version: Long = 0,
    val last: DriverClock? = null, val activeMillis: Long = 0, val manuallyReviewedMillis: Long = 0,
    val gaps: List<DriverShiftGap> = emptyList(), val receipts: List<DriverShiftReceipt> = emptyList(),
) {
    init {
        require(listOf(id, owner, origin, vehicle).all { it.isNotBlank() } && version >= 0 && activeMillis >= 0 && manuallyReviewedMillis >= 0)
        require(gaps.size <= 50 && receipts.size <= 50 && gaps.map { it.id }.toSet().size == gaps.size)
        require(receipts.map { it.id }.toSet().size == receipts.size)
        ZoneId.of(timezone)
        require((state == DriverShiftState.READY) == (last == null))
    }
    val knownActiveSeconds: Long get() = activeMillis / 1000
    val completeWorkSeconds: Long? get() = knownActiveSeconds.takeIf { gaps.none { gap -> gap.reviewedWorkMillis == null } }
    fun apply(command: DriverShiftCommand, policy: DriverClockPolicy): DriverShiftTransition {
        val fingerprint = command.fingerprint()
        receipts.find { it.id == command.id }?.let {
            check(it.fingerprint == fingerprint) { "driver-operation-conflict" }
            return DriverShiftTransition(this, it, true)
        }
        check(command.expectedVersion == version) { "driver-stale-shift" }
        val allowed = when (command.action) {
            DriverShiftAction.START -> state == DriverShiftState.READY
            DriverShiftAction.RESUME -> state == DriverShiftState.PAUSED
            DriverShiftAction.PAUSE, DriverShiftAction.CHECKPOINT -> state == DriverShiftState.RUNNING
            DriverShiftAction.END -> state in setOf(DriverShiftState.RUNNING, DriverShiftState.PAUSED)
            DriverShiftAction.RECOVER -> state in setOf(DriverShiftState.RUNNING, DriverShiftState.PAUSED)
            DriverShiftAction.REVIEW_GAP -> state in setOf(DriverShiftState.RECOVERY_REQUIRED, DriverShiftState.PAUSED)
        }
        check(allowed) { "driver-invalid-transition" }
        require(command.action == DriverShiftAction.REVIEW_GAP || (command.gapId == null && command.reviewedWorkMillis == null))
        val nextVersion = Math.addExact(version, 1)
        val receipt = DriverShiftReceipt(command.id, fingerprint, nextVersion)
        fun accepted(candidate: DriverShift): DriverShiftTransition = DriverShiftTransition(
            candidate.copy(version = nextVersion, receipts = (receipts + receipt).takeLast(50)), receipt, false,
        )
        fun gap(reason: String): DriverShiftTransition {
            check(gaps.size < 50) { "driver-gap-review-required" }
            return accepted(copy(state = DriverShiftState.RECOVERY_REQUIRED, last = command.clock,
                gaps = gaps + DriverShiftGap(command.id, reason, requireNotNull(last), command.clock)))
        }
        if (command.action == DriverShiftAction.REVIEW_GAP) {
            val target = gaps.singleOrNull { it.id == command.gapId } ?: error("driver-gap-missing")
            check(target.reviewedWorkMillis == null) { "driver-gap-already-reviewed" }
            require(command.reviewedWorkMillis != null || state == DriverShiftState.RECOVERY_REQUIRED)
            val extra = command.reviewedWorkMillis ?: 0L
            return accepted(copy(state = DriverShiftState.PAUSED, last = command.clock,
                activeMillis = Math.addExact(activeMillis, extra), manuallyReviewedMillis = Math.addExact(manuallyReviewedMillis, extra),
                gaps = gaps.map { if (it.id == target.id) it.copy(reviewedWorkMillis = command.reviewedWorkMillis) else it }))
        }
        if (command.action == DriverShiftAction.RECOVER && state == DriverShiftState.RUNNING) return gap("process-recovery")
        var elapsed = 0L
        last?.let { previous ->
            if (previous.bootId != command.clock.bootId) return gap("boot-changed")
            val mono = command.clock.elapsedMillis.toBigInteger() - previous.elapsedMillis.toBigInteger()
            val wall = command.clock.utcMillis.toBigInteger() - previous.utcMillis.toBigInteger()
            if (mono < BigInteger.ZERO || wall < BigInteger.ZERO || (wall - mono).abs() > policy.wallToleranceMillis.toBigInteger()) return gap("clock-discontinuity")
            if (state == DriverShiftState.RUNNING && mono > policy.maximumCheckpointGapMillis.toBigInteger()) return gap("checkpoint-gap")
            elapsed = mono.driverLongExact()
        }
        val worked = if (state == DriverShiftState.RUNNING) Math.addExact(activeMillis, elapsed) else activeMillis
        val nextState = when (command.action) {
            DriverShiftAction.START, DriverShiftAction.RESUME, DriverShiftAction.CHECKPOINT -> DriverShiftState.RUNNING
            DriverShiftAction.PAUSE, DriverShiftAction.RECOVER -> DriverShiftState.PAUSED
            DriverShiftAction.END -> DriverShiftState.CLOSED
            DriverShiftAction.REVIEW_GAP -> error("handled above")
        }
        return accepted(copy(state = nextState, last = command.clock, activeMillis = worked))
    }
}
