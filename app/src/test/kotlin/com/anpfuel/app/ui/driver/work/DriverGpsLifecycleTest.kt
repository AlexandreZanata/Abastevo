package com.anpfuel.app.ui.driver.work

import com.anpfuel.data.local.entity.DriverGpsObservationEntity
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class DriverGpsLifecycleTest {
    private class Harness {
        var owner = "a"
        var attached = 0
        var detached = 0
        var eligible = true
        var failWrite = false
        var failAttach = false
        var finalWrite: CompletableDeferred<Unit>? = null
        val rows = mutableMapOf<String, DriverGpsObservationEntity>()
        val lifecycle = DriverGpsLifecycle(
            currentOwner = { owner }, eligible = { _, _ -> eligible },
            persist = { row ->
                if (failWrite) error("write failed")
                if (row.toAtMillis != row.fromAtMillis) finalWrite?.await()
                rows[row.observationId] = row
            },
            attach = { attached++; if (failAttach) error("attach failed") }, detach = { detached++ },
        )
    }

    @Test fun `duplicate start stop and failed admission do not add listeners`() = runTest {
        val h = Harness()
        h.eligible = false
        assertFalse(h.lifecycle.start("a", "shift", "id", 10))
        assertEquals(0, h.attached)
        h.eligible = true
        assertTrue(h.lifecycle.start("a", "shift", "id", 10))
        assertFalse(h.lifecycle.start("a", "shift", "second", 11))
        assertEquals(1, h.attached)
        h.lifecycle.stop(20)
        h.lifecycle.stop(21)
        assertEquals(1, h.detached)
        assertEquals(1, h.rows.size)
        val row = h.rows.getValue("id")
        assertFalse(row.complete)
        assertTrue(row.hasGap)
        assertNull(row.metres)
    }

    @Test fun `stop detaches first and waits for durable final aggregate`() = runTest {
        val h = Harness()
        assertTrue(h.lifecycle.start("a", "shift", "id", 10))
        assertTrue(h.lifecycle.addMetres(42.5))
        val gate = CompletableDeferred<Unit>()
        h.finalWrite = gate
        val finish = async { h.lifecycle.stop(20) }
        runCurrent()
        assertEquals(1, h.detached)
        assertFalse(finish.isCompleted)
        assertNull(h.rows.getValue("id").metres)
        gate.complete(Unit)
        finish.await()
        assertEquals(42L, h.rows.getValue("id").metres)
        assertFalse(h.rows.getValue("id").complete)
    }

    @Test fun `owner switch never rebinds observation or accepts another step`() = runTest {
        val h = Harness()
        assertTrue(h.lifecycle.start("a", "shift", "id", 10))
        assertTrue(h.lifecycle.addMetres(10.0))
        h.owner = "b"
        assertFalse(h.lifecycle.addMetres(50.0))
        h.lifecycle.stop(20)
        assertEquals("a", h.rows.getValue("id").ownerAccount)
        assertEquals(10L, h.rows.getValue("id").metres)
        assertTrue(h.rows.values.none { it.ownerAccount == "b" })
        assertEquals(1, h.detached)
    }

    @Test fun `initial failure never attaches and final failure still detaches`() = runTest {
        val h = Harness()
        h.failWrite = true
        assertTrue(runCatching { h.lifecycle.start("a", "shift", "id", 10) }.isFailure)
        assertEquals(0, h.attached)
        h.failWrite = false
        assertTrue(h.lifecycle.start("a", "shift", "id", 10))
        h.failWrite = true
        assertTrue(runCatching { h.lifecycle.stop(20) }.isFailure)
        assertEquals(1, h.detached)
        assertFalse(h.rows.getValue("id").complete)
        assertTrue(h.rows.getValue("id").hasGap)
    }

    @Test fun `attach failure and destroy release listener while preserving incomplete checkpoint`() = runTest {
        val h = Harness()
        h.failAttach = true
        assertTrue(runCatching { h.lifecycle.start("a", "shift", "id", 10) }.isFailure)
        assertEquals(1, h.detached)
        assertFalse(h.rows.getValue("id").complete)
        h.failAttach = false
        assertTrue(h.lifecycle.start("a", "shift", "next", 20))
        h.lifecycle.destroy()
        assertEquals(2, h.detached)
        assertFalse(h.lifecycle.addMetres(10.0))
        assertTrue(h.rows.getValue("next").hasGap)
    }
}
