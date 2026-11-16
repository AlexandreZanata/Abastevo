package com.anpfuel.domain.enterprise.tracking

import java.time.Instant

/*
 * EMP-GPS-010 (B-BR-EMP-544..547, EDR-354..356): the session-level interruptions of a stored session, the cause of an
 * interruption that the device observes, the precision a fix may claim and the fix-level gap that a new recording run
 * must not inherit. Pure; no coordinate ever appears here.
 */

/** One interval in which a session that had recorded was not recording (BR-GPS-024): a state, a cause and bounds, never a position. */
class TrackingInterruption(val state: TrackingSessionState, val cause: TrackingCause, val start: Instant, val end: Instant?) {
    init { require(end == null || !end.isBefore(start)) }
    override fun toString(): String = "EnterpriseTrackingInterruption(redacted)"
}

object TrackingInterruptions {
    /**
     * Every entry that leaves `RECORDING` for `SUSPENDED` or `INTERRUPTED` opens an interval with its cause; the next
     * `RECORDING` entry (a resume) or the end closes it; refused attempts in between keep it open; a reduction is not one.
     */
    fun of(transitions: List<StoredTrackingTransition>): List<TrackingInterruption> {
        val out = mutableListOf<TrackingInterruption>()
        var open: StoredTrackingTransition? = null
        transitions.forEachIndexed { i, row ->
            val current = open
            if (current != null) {
                if (row.state == TrackingSessionState.RECORDING || row.state == TrackingSessionState.ENDED) {
                    out += TrackingInterruption(current.state, requireNotNull(current.cause), current.at, row.at)
                    open = null
                }
            } else if (i > 0 && transitions[i - 1].state == TrackingSessionState.RECORDING &&
                (row.state == TrackingSessionState.SUSPENDED || row.state == TrackingSessionState.INTERRUPTED)) {
                open = row
            }
        }
        open?.let { out += TrackingInterruption(it.state, requireNotNull(it.cause), it.at, null) }
        return out.toList()
    }

    /**
     * BR-GPS-011/012 and §8.2: the cause of an interruption of a recording session that nothing collects for, in the order
     * the device can tell it: another boot, then no location permission, then a notification that cannot be seen.
     */
    fun observedCause(boots: Collection<String>, bootId: String, permission: LocationPermission?, notificationVisible: Boolean?): TrackingCause =
        when {
            boots.any { it != bootId } -> TrackingCause.DEVICE_RESTARTED
            permission == LocationPermission.NONE -> TrackingCause.PERMISSION_REVOKED
            notificationVisible == false -> TrackingCause.NOTIFICATION_BLOCKED
            else -> TrackingCause.PROCESS_STOPPED
        }

    /**
     * BR-GPS-007: the precision a fix may claim, never finer than its recording period nor than the permission granted with
     * the report ([granted] null when not observed); null when nothing may be recorded.
     */
    fun fixPrecision(period: LocationPermission?, granted: LocationPermission?): LocationPermission? = when {
        period == null || period == LocationPermission.NONE || granted == LocationPermission.NONE -> null
        granted == LocationPermission.APPROXIMATE -> LocationPermission.APPROXIMATE
        else -> period
    }

    /** B-BR-EMP-545: a fix-level gap still open from a run that ended before [runStart] closes at that run's end, never carried. */
    fun enterRun(buffer: SessionBuffer, runStart: Instant, previousEnd: Instant?): SessionBuffer {
        val open = buffer.openGap ?: return buffer
        if (!open.start.isBefore(runStart)) return buffer
        return TrackingSampler.closePeriod(buffer, minOf(previousEnd ?: runStart, runStart))
    }
}
