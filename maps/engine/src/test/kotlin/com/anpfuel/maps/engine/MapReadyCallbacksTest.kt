package com.anpfuel.maps.engine

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class MapReadyCallbacksTest {
    @Test
    fun `release drops pending initialization callbacks`() {
        val ready = MapReadyCallbacks()
        var calls = 0
        ready.add { calls++ }
        ready.invalidate()
        ready.take().forEach { it() }
        assertEquals(0, calls)
    }

    @Test
    fun `already queued callbacks cannot outlive their host`() {
        val ready = MapReadyCallbacks()
        var oldCalls = 0
        var newCalls = 0
        ready.add { oldCalls++ }
        val queued = ready.take()
        ready.invalidate()
        ready.add { newCalls++ }
        queued.forEach { it() }
        ready.take().forEach { it() }
        assertEquals(0, oldCalls)
        assertEquals(1, newCalls)
    }

    @Test
    fun `concurrent initialization retains each current callback once`() {
        val ready = MapReadyCallbacks()
        val threads = List(12) { Thread { ready.add { } } }
        threads.forEach { it.start() }
        threads.forEach { it.join() }
        assertEquals(12, ready.take().size)
        assertEquals(0, ready.take().size)
    }
}
