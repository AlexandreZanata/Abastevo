package com.anpfuel.domain.driver

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/** Declared synthetic policy, not a production device calibration or default. */
class DriverGpsQualityTest {
    private val policy = DriverGpsPolicy(1, 30.0, 10000, 10000, 100.0, 5.0)
    private fun clock(ms: Long, boot: String = "b") = DriverClock(boot, ms, 100000 + ms)
    private fun fix(lon: Double, ms: Long, accuracy: Double = 5.0, boot: String = "b") = DriverGpsFix(0.0, lon, accuracy, clock(ms, boot))
    private fun empty() = DriverGpsQuality.empty(DriverDistanceScope("owner", "origin", "car", "session"), policy)
    @Test fun `stationary drift does not create kilometres and slow movement eventually accumulates`() {
        var track = empty().append(fix(0.0, 0), clock(0))
        repeat(100) { i -> track = track.append(fix(if (i % 2 == 0) 0.000001 else -0.000001, (i+1)*100L), clock((i+1)*100L)) }
        assertEquals(0L, track.estimate().distanceMetres)
        assertFalse(track.estimate().hasGap)
        track = track.append(fix(0.0001, 11000), clock(11000))
        assertEquals(11L, track.estimate().distanceMetres)
    }
    @Test fun `jumps stale inaccurate future invalid and reordered fixes cannot add distance`() {
        val initial = empty().append(fix(0.0, 0), clock(0))
        val moved = initial.append(fix(0.001, 2000), clock(2000))
        assertEquals(111L, moved.estimate().distanceMetres)
        val cases = listOf(
            fix(1.0, 3000) to clock(3000), fix(0.002, 1000) to clock(3000),
            fix(0.002, 3000, 100.0) to clock(3000), fix(0.002, 3000) to clock(14000),
            fix(0.002, 4000) to clock(3000), fix(Double.NaN, 3000) to clock(3000),
        )
        cases.forEach { (bad, now) ->
            val result = moved.append(bad, now).estimate()
            assertEquals(111L, result.distanceMetres)
            assertEquals(1L, result.rejected.values.sum())
            assertFalse(result.complete)
        }
        assertEquals("DriverGpsFix(private)", fix(1.234, 0).toString())
        assertFalse(moved.toString().contains("latitude"))
    }
    @Test fun `tunnel and reboot gaps create new anchors without inventing travel`() {
        val moved = empty().append(fix(0.0, 0), clock(0)).append(fix(0.001, 2000), clock(2000))
        val tunnel = moved.append(fix(0.1, 20000), clock(20000))
        assertEquals(111L, tunnel.estimate().distanceMetres)
        assertTrue(tunnel.estimate().hasGap)
        val reboot = moved.append(fix(0.1, 0, boot = "next"), clock(0, "next"))
        assertEquals(111L, reboot.estimate().distanceMetres)
        assertTrue(reboot.estimate().hasGap)
        assertEquals(222L, reboot.append(fix(0.101, 2000, boot = "next"), clock(2000, "next")).estimate().distanceMetres)
    }
    @Test fun `antimeridian uses the short geodesic rather than a world circumference`() {
        val track = empty().append(fix(179.999, 0), clock(0)).append(fix(-179.999, 5000), clock(5000))
        assertEquals(222L, track.estimate().distanceMetres)
    }
}
