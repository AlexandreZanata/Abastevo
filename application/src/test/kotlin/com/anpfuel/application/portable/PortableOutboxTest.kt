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
        assertEquals(1000L, cmd.nextEligibleTick)

        cmd = PortableOutbox.markFailed(cmd, tickMillis = 1000L)
        assertEquals(3000L, cmd.nextEligibleTick)

        repeat(30) { cmd = PortableOutbox.markFailed(cmd, tickMillis = 0L) }
        assertTrue(cmd.nextEligibleTick <= PortableOutbox.MAX_DELAY_TICKS)
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
