package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.ReportingZones
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fleet.AssetId
import com.anpfuel.domain.enterprise.fleet.AssetRef
import com.anpfuel.domain.enterprise.meter.MeterKind
import com.anpfuel.domain.enterprise.meter.MeterRef
import com.anpfuel.domain.enterprise.meter.MeterValue
import com.anpfuel.domain.enterprise.meter.ReadingRef
import com.anpfuel.domain.enterprise.meter.SegmentRef
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertSame
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class EnterpriseTripTimeTest {
    private fun id(n: Int) = "5e000000-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val org = id(1)
    private val asset = AssetRef(org, AssetId.parse(id(21)))
    private fun reading(n: Int) = ReadingRef(org, SegmentRef(org, MeterRef(org, asset, id(51), MeterKind.ODOMETER), id(61)), id(n))
    private fun observed(n: Int, units: Long) = TripMeterEvidence.Observed(reading(n), MeterValue.of(MeterKind.ODOMETER, units))
    private fun at(raw: String, zone: String = "America/Cuiaba", offset: Long = -14400) =
        TimeDeclaration(EnterpriseTime.parseInstant(raw), zone, offset)
    private fun start(clock: TripRunClock? = null) = Trip.start(org, TripStartInput(TripRef(org, id(11)), MembershipRef(org, id(41)),
        asset, EmployeeRef(org, id(31)), observed(71, 184_320_000), at("2026-11-14T12:00:00Z"), ReportingZones("America/Cuiaba"), clock))
    private fun finishInput(declared: TimeDeclaration = at("2026-11-14T15:00:00Z"), clock: TripRunClock? = null) =
        TripFinishInput(MembershipRef(org, id(41)), observed(72, 184_610_000), declared, clock)

    @Test fun `run clocks follow the boot identity profile`() {
        val clock = TripRunClock("boot-a1", 7)
        assertEquals("boot-a1", clock.bootId)
        assertEquals(7L, clock.elapsedMillis)
        assertEquals(TripRunClock("boot-a1", 7), clock)
        assertEquals(TripRunClock("boot-a1", 7).hashCode(), clock.hashCode())
        assertNotEquals(TripRunClock("boot-a1", 8), clock)
        assertNotEquals(TripRunClock("boot-b2", 7), clock)
        assertNotEquals(clock as Any, "boot-a1")
        for (boot in listOf("", "b".repeat(65), "boot 1", "bóot", "boot\n")) {
            assertEquals(TripFailure.INVALID, assertThrows(TripDomainException::class.java) { TripRunClock(boot, 0) }.failure)
        }
        assertEquals(TripFailure.INVALID, assertThrows(TripDomainException::class.java) { TripRunClock("boot-a1", -1) }.failure)
        assertFalse(clock.toString().contains("boot-a1"))
    }

    @Test fun `one trip crosses midnight and zones without changing its basis`() {
        val started = start(TripRunClock("boot-a1", 1_000))
        assertFalse(started.crossing.finished)
        assertNull(started.crossing.endDay)
        assertEquals("2026-11-14", started.crossing.day)
        assertEquals(TripElapsedStatus.NOT_FINISHED, started.elapsed(0).status)
        val done = started.finish(org, finishInput(at("2026-11-15T04:10:00Z", "America/Sao_Paulo", -10800)))
        val crossing = done.crossing
        assertTrue(crossing.finished && crossing.crossesMidnight && crossing.entryZoneChanged && crossing.offsetChanged)
        assertEquals("2026-11-15", crossing.endDay)
        assertEquals(started.reporting, done.reporting)
        assertEquals(started.reference, done.reference)
        assertSame(started.started, done.started)
        assertEquals("America/Cuiaba", done.started.declared.entryZone)
        assertEquals("America/Sao_Paulo", done.finished!!.declared.entryZone)
        assertEquals(TripRunClock("boot-a1", 1_000), done.started.clock)
        assertNull(done.finished!!.clock)
        val elapsed = done.elapsed(0)
        assertEquals(TripElapsedStatus.DECLARED, elapsed.status)
        assertEquals(58_200L, elapsed.seconds)
        assertEquals(0, elapsed.nanos)
        assertNull(elapsed.gap)
    }

    @Test fun `elapsed time is never invented`() {
        val started = start(TripRunClock("boot-a1", 0))
        assertEquals(TripFailure.INVALID, assertThrows(TripDomainException::class.java) { started.elapsed(-1) }.failure)
        val reboot = started.finish(org, finishInput(clock = TripRunClock("boot-b2", 10_800_000))).elapsed(2_000)
        assertEquals(TripElapsedStatus.GAP, reboot.status)
        assertEquals(TripGapReason.BOOT_CHANGED, reboot.gap)
        assertNull(reboot.seconds)
        assertNull(reboot.nanos)
        val jump = started.finish(org, finishInput(clock = TripRunClock("boot-a1", 21_600_000))).elapsed(2_000)
        assertEquals(TripGapReason.CLOCK_DISCONTINUITY, jump.gap)
        val confirmed = started.finish(org, finishInput(clock = TripRunClock("boot-a1", 10_800_000))).elapsed(0)
        assertEquals(TripElapsedStatus.CONFIRMED, confirmed.status)
        assertEquals(10_800L, confirmed.seconds)
        assertFalse(confirmed.toString().contains("10800"))
    }
}
