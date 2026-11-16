package com.anpfuel.domain.enterprise.tracking

import java.time.Instant
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** EMP-GPS-003 sampling, filter, gaps and the sealed buffer (B-BR-EMP-501..505, EDR-325..327). Synthetic positions only. */
class TrackingSamplerTest {
    private fun id(n: Int) = "9b000000-0000-4000-8000-%012d".format(n)
    private val start = Instant.parse("2026-11-15T12:00:00Z")
    private var elapsed = 1_000_000L
    private var wall = start
    private val P = LocationPermission.PRECISE
    private fun empty() = SessionBuffer.empty(id(11), id(21), "boot-7")
    private fun obs(lat: Double = -15.6, lon: Double = -56.1, accuracy: Float? = 8f, speed: Float? = 14f, bearing: Float? = 270f, mock: Boolean = false,
        age: Long = 0) = TrackingObservation(lat, lon, accuracy, speed, bearing, mock, elapsed - age)
    private fun advance(millis: Long) { elapsed += millis; wall = wall.plusMillis(millis) }
    private fun observe(b: SessionBuffer, o: TrackingObservation, precision: LocationPermission = P, economy: Boolean = false) =
        TrackingSampler.observe(b, o, precision, economy, start, elapsed, wall)
    /** ~0.000126° of longitude at 15.6° S is about 13.5 m. */
    private fun east(metres: Double) = -56.1 + metres / 108_000.0

    @Test fun `the filter counts mock, stale, inaccurate and impossible fixes and never records them`() {
        var b = empty()
        advance(1_000)
        b = observe(b, obs(mock = true)).also { assertEquals(TrackingObservationOutcome.DROPPED, it.second) }.first
        b = observe(b, obs(age = 15_001)).also { assertEquals(TrackingObservationOutcome.DROPPED, it.second) }.first
        b = observe(b, obs(accuracy = null)).also { assertEquals(TrackingObservationOutcome.DROPPED, it.second) }.first
        b = observe(b, obs(accuracy = 50.5f)).also { assertEquals(TrackingObservationOutcome.DROPPED, it.second) }.first
        assertEquals(TrackingObservationOutcome.IGNORED, observe(b, obs(lat = Double.NaN)).second)
        assertEquals(TrackingObservationOutcome.IGNORED, observe(b, obs(lon = 181.0)).second)
        assertEquals(TrackingDropped(accuracy = 2, stale = 1, jump = 0, mock = 1), b.dropped)
        assertTrue(b.pending.isEmpty())
        // A report taken before the recording period started is stale even if it is young.
        val early = TrackingSampler.observe(empty(), obs(age = 2_000), P, false, wall.minusMillis(1_000), elapsed, wall)
        assertEquals(TrackingObservationOutcome.DROPPED, early.second); assertEquals(1L, early.first.dropped.stale)
        b = observe(b, obs()).first
        assertEquals(1, b.pending.size)
        // 2 km in 10 s is 200 m/s: a jump, counted and not recorded; an approximate period has no jump or accuracy check.
        advance(10_000)
        b = observe(b, obs(lon = east(2_000.0))).also { assertEquals(TrackingObservationOutcome.DROPPED, it.second) }.first
        assertEquals(1L, b.dropped.jump)
        val coarse = TrackingSampler.observe(empty(), obs(accuracy = 1_500f), LocationPermission.APPROXIMATE, false, start, elapsed, wall).first
        assertTrue(coarse.pending.single().coarse); assertEquals(1500, coarse.pending.single().accuracyMetres)
    }

    @Test fun `a fix's instant is the device clock minus its monotonic age, in batch units`() {
        advance(30_000)
        val b = observe(empty(), obs(lat = -15.60000004, lon = -56.10000006, accuracy = 7.2f, speed = 13.894f, bearing = 359.7f, age = 2_500)).first
        val fix = b.pending.single()
        assertEquals(wall.minusMillis(2_500), fix.at)
        assertEquals(elapsed - 2_500, fix.elapsedMillis)
        assertEquals(listOf(-156000000, -561000001, 8, 1389, 0), listOf(fix.latE7, fix.lonE7, fix.accuracyMetres, fix.speedCmPerSecond, fix.bearingDegrees))
        assertEquals("EnterpriseBufferedFix(redacted)", fix.toString())
    }

    @Test fun `sampling-v1 records the first fix, then by time while moving, by distance while slow and a heartbeat when still`() {
        var b = observe(empty(), obs()).first
        var lon = -56.1
        fun moving(millis: Long, metres: Double, bearing: Float = 270f, economy: Boolean = false): TrackingObservationOutcome {
            advance(millis); lon = east((lon + 56.1) * 108_000.0 + metres)
            val r = observe(b, obs(lon = lon, bearing = bearing), economy = economy); b = r.first; return r.second
        }
        assertEquals(TrackingObservationOutcome.ACCEPTED_NOT_RECORDED, moving(5_000, 70.0))
        assertEquals(TrackingObservationOutcome.RECORDED, moving(5_000, 70.0), "10 s while moving")
        assertEquals(TrackingObservationOutcome.ACCEPTED_NOT_RECORDED, moving(4_000, 50.0, bearing = 330f), "a turn sooner than 5 s")
        assertEquals(TrackingObservationOutcome.RECORDED, moving(1_000, 14.0, bearing = 330f), "a 60° turn after 5 s")
        assertEquals(TrackingObservationOutcome.ACCEPTED_NOT_RECORDED, moving(10_000, 140.0, bearing = 330f, economy = true), "economy waits 30 s")
        assertEquals(TrackingObservationOutcome.ACCEPTED_NOT_RECORDED, moving(10_000, 140.0, bearing = 30f, economy = true), "no turn trigger in economy")
        assertEquals(TrackingObservationOutcome.RECORDED, moving(10_000, 140.0, bearing = 30f, economy = true))
        // Slow (below 1.5 m/s): only 50 m from the last recorded fix, or the heartbeat.
        advance(20_000); b = observe(b, obs(lon = east((lon + 56.1) * 108_000.0 + 20.0), speed = 1f)).also {
            assertEquals(TrackingObservationOutcome.ACCEPTED_NOT_RECORDED, it.second) }.first
        advance(20_000); b = observe(b, obs(lon = east((lon + 56.1) * 108_000.0 + 55.0), speed = 1f)).also {
            assertEquals(TrackingObservationOutcome.RECORDED, it.second) }.first
        val last = b.lastRecorded!!
        advance(119_000); b = observe(b, obs(lat = last.latE7 / 1e7, lon = last.lonE7 / 1e7, speed = 0f)).also {
            assertEquals(TrackingObservationOutcome.ACCEPTED_NOT_RECORDED, it.second) }.first
        advance(1_000); b = observe(b, obs(lat = last.latE7 / 1e7, lon = last.lonE7 / 1e7, speed = 0f)).also {
            assertEquals(TrackingObservationOutcome.RECORDED, it.second, "heartbeat") }.first
        // The first fix plus the 10 s, turn, economy, slow-distance and heartbeat records.
        assertEquals(6, b.pending.size); assertEquals(6L, b.recorded)
        // Approximate: the heartbeat only.
        var c = TrackingSampler.observe(empty(), obs(accuracy = 900f), LocationPermission.APPROXIMATE, false, start, elapsed, wall).first
        advance(60_000)
        c = TrackingSampler.observe(c, obs(lon = east(900.0), accuracy = 900f), LocationPermission.APPROXIMATE, false, start, elapsed, wall).first
        assertEquals(1, c.pending.size)
        advance(60_000)
        c = TrackingSampler.observe(c, obs(lon = east(900.0), accuracy = 900f), LocationPermission.APPROXIMATE, false, start, elapsed, wall).first
        assertEquals(2, c.pending.size)
    }

    @Test fun `gaps open after 180 seconds with their reason, close at the next accepted fix and invent nothing`() {
        advance(1_000)
        var b = observe(empty(), obs()).first
        val lastAt = b.lastAcceptedAt!!
        advance(180_000)
        b = TrackingSampler.tick(b, wall, providerEnabled = true, periodStart = start)
        assertNull(b.openGap, "exactly 180 s is not yet a gap")
        advance(1)
        b = TrackingSampler.tick(b, wall, true, start)
        assertEquals(TrackingGapReason.SIGNAL_LOST, b.openGap!!.reason); assertEquals(lastAt, b.openGap!!.start)
        assertEquals(1, b.pending.size, "a gap adds no fix")
        assertEquals(1, TrackingSampler.tick(b, wall.plusSeconds(600), true, start).gaps.size, "an open gap is not reopened")
        advance(5_000)
        b = observe(b, obs(lon = east(40.0))).first
        assertEquals(wall, b.gaps.single().end); assertNull(b.openGap)
        // Only mock reports since the last accepted fix: MOCK_LOCATION; a disabled provider: PROVIDER_DISABLED at once.
        advance(5_000); b = observe(b, obs(mock = true)).first
        advance(181_000); b = TrackingSampler.tick(b, wall, true, start)
        assertEquals(TrackingGapReason.MOCK_LOCATION, b.openGap!!.reason)
        b = TrackingSampler.closePeriod(b, wall)
        assertEquals(wall, b.gaps.last().end)
        advance(1_000); b = observe(b, obs(lon = east(60.0))).first
        b = TrackingSampler.tick(b, wall.plusSeconds(1), false, start)
        assertEquals(TrackingGapReason.PROVIDER_DISABLED, b.openGap!!.reason)
        // A session that never got a fix has its gap from the period start.
        val never = TrackingSampler.tick(empty(), start.plusSeconds(181), true, start)
        assertEquals(start, never.openGap!!.start)
    }

    @Test fun `the buffer is bounded, canonical and strict and a batch takes the oldest continuous run`() {
        var b = empty()
        repeat(TrackingSamplingPolicy.MAX_PENDING) { i ->
            advance(120_000); b = observe(b, obs(lon = east(i.toDouble()), speed = 0f)).first
        }
        assertEquals(TrackingSamplingPolicy.MAX_PENDING, b.pending.size)
        advance(120_000)
        val (full, outcome) = observe(b, obs(lon = east(5_000.0), speed = 0f))
        assertEquals(TrackingObservationOutcome.OVERFLOW, outcome); assertEquals(1L, full.overflow)
        assertEquals(TrackingSamplingPolicy.MAX_PENDING, full.pending.size, "nothing pending is dropped")
        assertEquals(TrackingBatchOperations.MAX_FIXES, TrackingBatching.nextRun(full).size)
        val book = TrackingBufferBook.empty(id(31), id(41)).with(full)
        val decoded = TrackingBufferBook.decode(book.canonicalBytes)
        assertArrayEquals(book.canonicalBytes, decoded.canonicalBytes)
        assertEquals(full.pending.size, decoded.session(id(11))!!.pending.size)
        assertThrows(IllegalArgumentException::class.java) { decoded.requireBinding(id(32), id(41)) }
        val text = String(book.canonicalBytes)
        for (tampered in listOf(text.replace("\"version\":1", "\"version\":2"), text.replace("\"recorded\":", "\"recorded\" :"),
            text.replace("tracking-buffer", "tracking-book"))) assertThrows(Exception::class.java) { TrackingBufferBook.decode(tampered.toByteArray()) }
        assertFalse(text.isEmpty())
        assertEquals("EnterpriseTrackingBufferBook(redacted)", book.toString())
        // A run stops at the start of a gap: no batch claims continuity across it.
        var g = observe(empty(), obs()).first
        advance(10_000); g = observe(g, obs(lon = east(140.0))).first
        advance(200_000); g = TrackingSampler.tick(g, wall, true, start)
        advance(1_000); g = observe(g, obs(lon = east(300.0))).first
        assertEquals(3, g.pending.size)
        assertEquals(2, TrackingBatching.nextRun(g).size)
        assertEquals(listOf(0L, 10_000L), TrackingBatching.fixes(TrackingBatching.nextRun(g)).map { it.offsetMillis })
        assertTrue(TrackingBatching.due(g, wall, force = true)); assertFalse(TrackingBatching.due(empty(), wall, force = true))
        assertTrue(TrackingBatching.due(g, g.pending.first().at.plusMillis(TrackingSamplingPolicy.SEAL_MILLIS), force = false))
        assertFalse(TrackingBatching.due(g, g.pending.first().at.plusMillis(TrackingSamplingPolicy.SEAL_MILLIS - 1), force = false))
    }
}
