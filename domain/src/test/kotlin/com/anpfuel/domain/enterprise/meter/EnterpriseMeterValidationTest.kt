package com.anpfuel.domain.enterprise.meter

import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fleet.AssetId
import com.anpfuel.domain.enterprise.fleet.AssetRef
import java.time.Instant
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseMeterValidationTest {
    private fun id(n: Int) = "8c000000-0000-4000-8000-" + n.toString().padStart(12, '0')
    private fun point(n: Int, value: Long, occurred: String, received: String) = MeterPoint(id(n), value, Instant.parse(occurred), Instant.parse(received))
    private val example = listOf(
        MeterSeriesEntry(point(200, 183_000_000, "2026-11-01T12:00:00Z", "2026-11-01T12:00:01Z"), ReadingState.VALID),
        MeterSeriesEntry(point(201, 183_400_000, "2026-11-03T11:00:00Z", "2026-11-03T11:00:05Z"), ReadingState.VALID),
        MeterSeriesEntry(point(202, 183_932_000, "2026-11-05T11:00:00Z", "2026-11-05T11:00:05Z"), ReadingState.VALID))

    @Test fun `default configuration is the documented one and redacts diagnostics`() {
        val policy = MeterPlausibility.DEFAULT
        assertEquals(150_000L, policy.distancePerHour)
        assertEquals(300L, policy.clockSkewSeconds)
        assertEquals(policy, MeterPlausibility.of(MeterPlausibility.DEFAULT_DISTANCE_PER_HOUR, MeterPlausibility.DEFAULT_CLOCK_SKEW_SECONDS))
        assertEquals("EnterpriseMeterValue(redacted)", policy.toString())
        assertEquals(MeterFailure.INVALID, assertThrows(MeterDomainException::class.java) { MeterPlausibility.of(19_999, 300) }.failure)
    }

    /** AC5: the app warns with the same validator and still saves the reading. */
    @Test fun `the device warning is a classification that never blocks saving`() {
        val typed = point(204, 148_190_000, "2026-11-06T12:00:00Z", "2026-11-06T12:00:30Z")
        val warning = MeterValidation.check(MeterPlausibility.DEFAULT, MeterKind.ODOMETER, example, typed)
        assertEquals(ReadingState.SUSPECT, warning.state)
        assertEquals(listOf(ReadingFlag.READING_BELOW_PREVIOUS), warning.flags)
        assertEquals(id(202), warning.previous)
        val org = id(1)
        val segment = SegmentRef(org, MeterRef(org, AssetRef(org, AssetId.parse(id(11))), id(21), MeterKind.ODOMETER), id(31))
        fun reading(n: Int, units: Long, op: Int, state: ReadingState) = Reading(ReadingRef(org, segment, id(n)), MeterValue.of(MeterKind.ODOMETER, units),
            TimeDeclaration(Instant.parse("2026-11-06T12:00:00Z"), "UTC", 0), Instant.parse("2026-11-06T12:00:30Z"),
            MembershipRef(org, id(41)), DeviceRef(org, id(51)), ReadingOrigin.STANDALONE, ReadingMethod.TYPED, OperationRef(org, id(op)), classification = state)
        val saved = MeterSeries.open(segment, reading(200, 183_000_000, 100, ReadingState.VALID)).append(reading(204, typed.value, 104, warning.state))
        assertEquals(ReadingState.SUSPECT, saved.state(ReadingRef(org, segment, id(204))))
    }

    @Test fun `examples A and B and the retroactive intervals`() {
        val r3 = point(203, 183_610_000, "2026-11-04T15:30:00Z", "2026-11-06T09:00:00Z")
        val v = MeterValidation.check(MeterPlausibility.DEFAULT, MeterKind.ODOMETER, example, r3)
        assertEquals(MeterVerdict(ReadingState.VALID, emptyList(), true, id(201), id(202)), v)
        assertEquals(532_000L, (r3.value - example[1].point.value) + (example[2].point.value - r3.value))
    }

    @Test fun `inputs are copied and never mutated`() {
        val flags = mutableListOf(ReadingFlag.READING_BELOW_PREVIOUS)
        val series = example.toMutableList().apply { add(MeterSeriesEntry(point(204, 148_190_000, "2026-11-06T12:00:00Z", "2026-11-06T12:00:30Z"), ReadingState.SUSPECT, flags)) }
        val snapshot = series.toList()
        assertEquals(emptyList<MeterVerdictChange>(), MeterValidation.reevaluate(MeterPlausibility.DEFAULT, MeterKind.ODOMETER, series))
        assertEquals(snapshot, series)
        assertEquals(listOf(ReadingFlag.READING_BELOW_PREVIOUS), flags)
    }
}
