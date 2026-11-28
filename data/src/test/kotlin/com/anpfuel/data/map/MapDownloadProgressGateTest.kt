package com.anpfuel.data.map

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** B-BR-MAP-DL-PROGRESS-01 pacing contract. */
class MapDownloadProgressGateTest {

    @Test
    fun `stores every step of bytes even without time passing`() {
        val gate = MapDownloadProgressGate(totalBytes = 10_000, startBytes = 0, startMillis = 0, stepBytes = 1_000, intervalMillis = 60_000)
        assertFalse(gate.shouldStore(999, 0))
        assertTrue(gate.shouldStore(1_000, 0))
        assertFalse(gate.shouldStore(1_500, 0))
        assertTrue(gate.shouldStore(2_000, 0))
    }

    @Test
    fun `stores after the interval even for a slow trickle`() {
        val gate = MapDownloadProgressGate(totalBytes = 10_000, startBytes = 0, startMillis = 0, stepBytes = 1_000, intervalMillis = 500)
        assertFalse(gate.shouldStore(10, 499))
        assertTrue(gate.shouldStore(20, 500))
        assertFalse(gate.shouldStore(30, 900))
        assertTrue(gate.shouldStore(40, 1_000))
    }

    @Test
    fun `always stores the final byte`() {
        val gate = MapDownloadProgressGate(totalBytes = 10_000, startBytes = 9_990, startMillis = 0, stepBytes = 1_000, intervalMillis = 60_000)
        assertTrue(gate.shouldStore(10_000, 1))
    }

    @Test
    fun `never stores a value that goes backwards, repeats or passes the total`() {
        val gate = MapDownloadProgressGate(totalBytes = 10_000, startBytes = 4_000, startMillis = 0, stepBytes = 1, intervalMillis = 0)
        assertFalse(gate.shouldStore(4_000, 10))
        assertFalse(gate.shouldStore(3_000, 10))
        assertFalse(gate.shouldStore(10_001, 10))
        assertTrue(gate.shouldStore(5_000, 10))
        assertFalse(gate.shouldStore(4_500, 20))
    }

    @Test
    fun `a resumed download starts from the stored part`() {
        val gate = MapDownloadProgressGate(totalBytes = 8_000, startBytes = 3_000, startMillis = 0, stepBytes = 1_000, intervalMillis = 60_000)
        assertFalse(gate.shouldStore(3_500, 0))
        assertTrue(gate.shouldStore(4_000, 0))
    }

    @Test
    fun `defaults follow the screen poll period`() {
        assertEquals(500L, MapDownloadProgressGate.DEFAULT_INTERVAL_MILLIS)
        assertTrue(MapDownloadProgressGate.DEFAULT_STEP_BYTES in 64L * 1024..1024L * 1024)
    }
}
