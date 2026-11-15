package com.anpfuel.domain.enterprise.trip

import java.math.BigInteger
import java.time.Duration

/** The driver-shift boot identity profile (B-BR-DRV-07). */
private val bootPattern = Regex("^[A-Za-z0-9._-]{1,64}$")
private val nanosPerSecond = BigInteger.valueOf(1_000_000_000L)
private val nanosPerMilli = BigInteger.valueOf(1_000_000L)

/**
 * A device monotonic witness of one event (BR-TIME-005): a boot identity and the monotonic count in
 * milliseconds since that boot. It confirms or questions elapsed time; it never becomes the duration.
 */
class TripRunClock(val bootId: String, val elapsedMillis: Long) {
    init { if (!bootPattern.matches(bootId) || elapsedMillis < 0) throw invalid() }
    override fun equals(other: Any?): Boolean = other is TripRunClock && bootId == other.bootId && elapsedMillis == other.elapsedMillis
    override fun hashCode(): Int = 31 * bootId.hashCode() + elapsedMillis.hashCode()
    override fun toString(): String = "EnterpriseTripRunClock(redacted)"
}

/** Informational time-crossing view; the trip's reporting day is always its start day. */
class TripCrossing internal constructor(
    val finished: Boolean,
    val day: String,
    val endDay: String?,
    val crossesMidnight: Boolean,
    val entryZoneChanged: Boolean,
    val offsetChanged: Boolean,
)

enum class TripElapsedStatus { NOT_FINISHED, DECLARED, CONFIRMED, GAP }
enum class TripGapReason { BOOT_CHANGED, CLOCK_DISCONTINUITY }

/** Elapsed time from instants only, or why there is none. */
class TripElapsed internal constructor(val status: TripElapsedStatus, val seconds: Long?, val nanos: Int?, val gap: TripGapReason?) {
    override fun toString(): String = "EnterpriseTripElapsed(redacted)"
}

/** The end day in the stored reporting zone and whether the device entry zone or offset changed (B-BR-EMP-276/277). */
val Trip.crossing: TripCrossing get() {
    val end = finished ?: return TripCrossing(false, reporting.day, null, false, false, false)
    val from = started.declared
    val to = end.declared
    return TripCrossing(true, reporting.day, endDay, endDay != reporting.day, from.entryZone != to.entryZone,
        from.deviceOffsetSeconds != to.deviceOffsetSeconds)
}

/**
 * Elapsed time from the start and finish instants only, checked against the run clocks with an explicit
 * tolerance in milliseconds (B-BR-EMP-278/279). A gap carries no value; nothing is invented.
 */
fun Trip.elapsed(toleranceMillis: Long): TripElapsed {
    if (toleranceMillis < 0) throw invalid()
    val end = finished ?: return TripElapsed(TripElapsedStatus.NOT_FINISHED, null, null, null)
    // Exact seconds and non-negative nanoseconds of two validated instants (finish is never before start).
    val between = Duration.between(started.declared.occurredAt, end.declared.occurredAt)
    val a = started.clock
    val b = end.clock
    if (a == null || b == null) return TripElapsed(TripElapsedStatus.DECLARED, between.seconds, between.nano, null)
    if (a.bootId != b.bootId) return TripElapsed(TripElapsedStatus.GAP, null, null, TripGapReason.BOOT_CHANGED)
    // Both counts are non-negative, so the difference cannot overflow.
    val mono = b.elapsedMillis - a.elapsedMillis
    val instant = BigInteger.valueOf(between.seconds).multiply(nanosPerSecond).add(BigInteger.valueOf(between.nano.toLong()))
    val drift = instant.subtract(BigInteger.valueOf(mono).multiply(nanosPerMilli)).abs()
    if (mono < 0 || drift > BigInteger.valueOf(toleranceMillis).multiply(nanosPerMilli)) {
        return TripElapsed(TripElapsedStatus.GAP, null, null, TripGapReason.CLOCK_DISCONTINUITY)
    }
    return TripElapsed(TripElapsedStatus.CONFIRMED, between.seconds, between.nano, null)
}
