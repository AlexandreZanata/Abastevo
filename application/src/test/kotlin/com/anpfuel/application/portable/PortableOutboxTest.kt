package com.anpfuel.application.portable

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class PortableOutboxTest {

    @Test
    fun enqueueAssignsRevisionOneAndKeepsFifoOrder() {
        var queue = emptyList<OutboxCommand>()
        queue = PortableOutbox.enqueue(queue, "cmd-1", "observation.submit", "{}")
        queue = PortableOutbox.enqueue(queue, "cmd-2", "observation.submit", "{}")

        assertEquals(listOf("cmd-1", "cmd-2"), queue.map { it.commandId })
        assertTrue(queue.all { it.revision == 1 && it.state == OutboxState.QUEUED })
    }

    @Test
    fun reenqueueSameCommandBumpsRevisionAndReplacesPayload() {
        var queue = PortableOutbox.enqueue(emptyList(), "cmd-1", "observation.submit", "{\"v\":1}")
        queue = PortableOutbox.enqueue(queue, "cmd-1", "observation.submit", "{\"v\":2}")

        assertEquals(1, queue.size)
        assertEquals(2, queue.single().revision)
        assertEquals("{\"v\":2}", queue.single().payload)
        assertEquals(OutboxState.QUEUED, queue.single().state)
    }

    @Test
    fun selectNextSkipsInFlightCancelledAndNotEligible() {
        val inFlight = PortableOutbox.markInFlight(
            OutboxCommand("cmd-1", "k", "p"), "nonce-1",
        )
        val cancelled = OutboxCommand("cmd-2", "k", "p", state = OutboxState.CANCELLED)
        val waiting = PortableOutbox.markFailed(
            OutboxCommand("cmd-3", "k", "p"), tickMillis = 0L,
        )
        val ready = OutboxCommand("cmd-4", "k", "p")
        val queue = listOf(inFlight, cancelled, waiting, ready)

        assertEquals("cmd-4", PortableOutbox.selectNext(queue, tickMillis = 0L, isCancelled = false)?.commandId)
        assertNull(PortableOutbox.selectNext(queue, tickMillis = 0L, isCancelled = true))
    }

    @Test
    fun retryBackoffIsExponentialWithCap() {
        var cmd = OutboxCommand("cmd-1", "k", "p")
        cmd = PortableOutbox.markFailed(cmd, tickMillis = 0L)
        assertEquals(1, cmd.attempts)
        // ±25% deterministic jitter around the 1000 base.
        assertTrue(cmd.nextEligibleTick in 750L..1250L)

        cmd = PortableOutbox.markFailed(cmd, tickMillis = 1000L)
        // Around the 2000 base, shifted by the tick.
        assertTrue(cmd.nextEligibleTick in 2500L..3500L)

        repeat(30) { cmd = PortableOutbox.markFailed(cmd, tickMillis = 0L) }
        assertEquals(PortableOutbox.MAX_DISPATCH_ATTEMPTS, cmd.attempts)
        assertEquals(Long.MAX_VALUE, cmd.nextEligibleTick)
    }

    // SEC06-D: attempts park instead of retrying forever; jitter
    // spreads a fleet deterministically per command/attempt.
    @Test
    fun attemptsParkAtCapAndJitterSpreads() {
        var cmd = OutboxCommand("cmd-park", "k", "p")
        repeat(PortableOutbox.MAX_DISPATCH_ATTEMPTS + 5) {
            cmd = PortableOutbox.markFailed(cmd, tickMillis = 0L)
        }
        assertEquals(PortableOutbox.MAX_DISPATCH_ATTEMPTS, cmd.attempts)
        assertEquals(Long.MAX_VALUE, cmd.nextEligibleTick)
        assertEquals(OutboxState.FAILED, cmd.state)

        val a = PortableOutbox.markFailed(OutboxCommand("cmd-a", "k", "p"), 0L).nextEligibleTick
        val b = PortableOutbox.markFailed(OutboxCommand("cmd-b", "k", "p"), 0L).nextEligibleTick
        assertTrue(a in 750L..1250L && b in 750L..1250L)
        // Same command and attempt always agree (test-stable, no RNG).
        assertEquals(
            a,
            PortableOutbox.markFailed(OutboxCommand("cmd-a", "k", "p"), 0L).nextEligibleTick,
        )
    }

    @Test
    fun ackRemovesExactRevisionAndIgnoresStaleAck() {
        var queue = PortableOutbox.enqueue(emptyList(), "cmd-1", "k", "{\"v\":1}")
        queue = PortableOutbox.enqueue(queue, "cmd-1", "k", "{\"v\":2}")

        val stale = PortableOutbox.markAcked(queue, "cmd-1", revision = 1)
        assertEquals(1, stale.size)

        val acked = PortableOutbox.markAcked(queue, "cmd-1", revision = 2)
        assertTrue(acked.isEmpty())
    }

    @Test
    fun cancelIsTerminalAndNeverSelected() {
        var queue = PortableOutbox.enqueue(emptyList(), "cmd-1", "k", "p")
        queue = PortableOutbox.cancel(queue, "cmd-1")

        assertEquals(OutboxState.CANCELLED, queue.single().state)
        assertNull(PortableOutbox.selectNext(queue, tickMillis = 0L, isCancelled = false))
    }

    @Test
    fun recordRoundTripPreservesEveryField() {
        val cmd = PortableOutbox.markInFlight(
            OutboxCommand("cmd-9", "observation.submit", "{\"price\":5999}", revision = 3, attempts = 2),
            nonce = "nonce-7",
        )
        val restored = PortableOutbox.fromRecord(PortableOutbox.toRecord(cmd))
        assertEquals(cmd, restored)
    }

    @Test
    fun recordIgnoresUnknownKeysButRefusesMissingOnes() {
        val record = PortableOutbox.toRecord(OutboxCommand("cmd-1", "k", "p")) + ("future_col" to "x")
        assertEquals("cmd-1", PortableOutbox.fromRecord(record).commandId)
        assertThrows(Exception::class.java) {
            PortableOutbox.fromRecord(record - "command_id")
        }
    }
}
