package com.anpfuel.domain.enterprise.tracking

import java.time.Instant
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertSame
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** EMP-GPS-010 pure interruption rules (B-BR-EMP-544..546, EDR-354/355). Synthetic identifiers and instants only. */
class TrackingInterruptionsTest {
    private fun id(n: Int) = "9b000000-0000-4000-8000-%012d".format(n)
    private val base = Instant.parse("2026-11-16T10:00:00Z")
    private fun at(minutes: Long): Instant = base.plusSeconds(minutes * 60)
    private fun row(state: TrackingSessionState, cause: TrackingCause?, minutes: Long, op: Int, permission: LocationPermission? = null,
        actor: TrackingActor = TrackingActor.DEVICE) = StoredTrackingTransition(state, cause, actor, permission, at(minutes), id(op))
    private val R = TrackingSessionState.RECORDING
    private val I = TrackingSessionState.INTERRUPTED

    @Test fun `missing gap - every interruption opens an interval with its cause that only a resume or the end closes`() {
        val rows = listOf(
            row(TrackingSessionState.NOT_STARTED, TrackingCause.NOT_STARTED, 0, 1, actor = TrackingActor.DRIVER),
            row(R, null, 0, 1, LocationPermission.PRECISE, TrackingActor.DRIVER),
            row(R, TrackingCause.PRECISION_REDUCED, 5, 2, LocationPermission.APPROXIMATE),
            row(I, TrackingCause.PROCESS_STOPPED, 10, 3),
            // A refused resume attempt keeps the interval open.
            row(I, TrackingCause.PERMISSION_DENIED, 12, 4, LocationPermission.NONE, TrackingActor.DRIVER),
            row(R, null, 15, 5, LocationPermission.PRECISE, TrackingActor.DRIVER),
            row(I, TrackingCause.DEVICE_RESTARTED, 20, 6),
            row(TrackingSessionState.ENDED, TrackingCause.TRIP_COMPLETED, 30, 7, actor = TrackingActor.TRIP),
        )
        val intervals = TrackingInterruptions.of(rows)
        assertEquals(listOf(TrackingCause.PROCESS_STOPPED, TrackingCause.DEVICE_RESTARTED), intervals.map { it.cause })
        assertEquals(listOf(at(10) to at(15), at(20) to at(30)), intervals.map { it.start to it.end })
        assertEquals(listOf(I, I), intervals.map { it.state })
        // Still interrupted: the interval stays open (an open gap, never closed by a guess).
        val open = TrackingInterruptions.of(rows.take(5)).single()
        assertNull(open.end); assertEquals(at(10), open.start)
        // A session that never recorded, or that recorded until its end, has no interruption; a reduction is not one.
        assertTrue(TrackingInterruptions.of(rows.take(1)).isEmpty())
        assertTrue(TrackingInterruptions.of(rows.take(3)).isEmpty())
        assertTrue(TrackingInterruptions.of(rows.take(3) + row(TrackingSessionState.ENDED, TrackingCause.TRIP_COMPLETED, 8, 9)).isEmpty())
        // A pause suspends: the interval carries its state and cause.
        val paused = TrackingInterruptions.of(rows.take(2) + row(TrackingSessionState.SUSPENDED, TrackingCause.TRIP_PAUSED, 4, 8))
        assertEquals(TrackingSessionState.SUSPENDED, paused.single().state); assertEquals(TrackingCause.TRIP_PAUSED, paused.single().cause)
        assertEquals("EnterpriseTrackingInterruption(redacted)", open.toString())
    }

    @Test fun `the observed cause names another boot first, then a missing permission, then a hidden notification`() {
        fun cause(boots: List<String>, permission: LocationPermission?, visible: Boolean?) =
            TrackingInterruptions.observedCause(boots, "boot-8", permission, visible)
        assertEquals(TrackingCause.DEVICE_RESTARTED, cause(listOf("boot-7"), LocationPermission.NONE, false))
        assertEquals(TrackingCause.DEVICE_RESTARTED, cause(listOf("boot-8", "boot-7"), LocationPermission.PRECISE, true))
        assertEquals(TrackingCause.PERMISSION_REVOKED, cause(listOf("boot-8"), LocationPermission.NONE, false))
        assertEquals(TrackingCause.NOTIFICATION_BLOCKED, cause(listOf("boot-8"), LocationPermission.APPROXIMATE, false))
        // A reduction found after a process death is not invented as a reduction: the process stopped.
        assertEquals(TrackingCause.PROCESS_STOPPED, cause(listOf("boot-8"), LocationPermission.APPROXIMATE, true))
        assertEquals(TrackingCause.PROCESS_STOPPED, cause(emptyList(), null, null))
    }

    @Test fun `precision overclaim - a fix never claims more precision than its period or the permission granted with it`() {
        val P = LocationPermission.PRECISE; val A = LocationPermission.APPROXIMATE; val N = LocationPermission.NONE
        assertEquals(P, TrackingInterruptions.fixPrecision(P, P))
        assertEquals(A, TrackingInterruptions.fixPrecision(P, A))
        assertEquals(A, TrackingInterruptions.fixPrecision(A, P), "a raise waits for the next resume")
        assertEquals(A, TrackingInterruptions.fixPrecision(A, A))
        assertNull(TrackingInterruptions.fixPrecision(P, N)); assertNull(TrackingInterruptions.fixPrecision(A, N))
        assertNull(TrackingInterruptions.fixPrecision(null, P)); assertNull(TrackingInterruptions.fixPrecision(N, P))
        assertEquals(P, TrackingInterruptions.fixPrecision(P, null), "not observed: the period decides")
    }

    @Test fun `missing gap - a fix-level gap left open by an earlier run closes at that run's end, never inside the next run`() {
        val runStart = at(20)
        val open = SessionBuffer.empty(id(11), id(21), "boot-7").let { TrackingSampler.tick(it, at(5), providerEnabled = false, periodStart = at(0)) }
        assertEquals(TrackingGapReason.PROVIDER_DISABLED, open.openGap!!.reason)
        val entered = TrackingInterruptions.enterRun(open, runStart, at(10))
        assertNull(entered.openGap); assertEquals(at(0) to at(10), entered.gaps.single().let { it.start to it.end })
        // Without a known previous end the gap closes at the new run's start, never later.
        assertEquals(runStart, TrackingInterruptions.enterRun(open, runStart, null).gaps.single().end)
        // A gap of the current run, and a buffer without an open gap, are left as they are.
        val current = SessionBuffer.empty(id(11), id(21), "boot-7").let { TrackingSampler.tick(it, at(25), providerEnabled = false, periodStart = runStart) }
        assertSame(current, TrackingInterruptions.enterRun(current, runStart, at(10)))
        assertSame(entered, TrackingInterruptions.enterRun(entered, runStart, at(10)))
    }
}
