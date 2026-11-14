package com.anpfuel.domain.enterprise.meter

import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fleet.AssetId
import com.anpfuel.domain.enterprise.fleet.AssetRef
import java.time.Instant
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseMeterEvidenceTest {
    private fun id(index: Int) = "8c000000-0000-4000-8000-" + index.toString().padStart(12, '0')
    private val org = id(1)
    private val other = id(2)
    private val asset = AssetRef(org, AssetId.parse(id(11)))
    private val meter = MeterRef(org, asset, id(21), MeterKind.ODOMETER)
    private val segment = SegmentRef(org, meter, id(31))
    private val at = Instant.parse("2026-11-01T00:00:00Z")
    private val member = MembershipRef(org, id(41))
    private val device = DeviceRef(org, id(51))
    private fun reading(index: Int, value: Long, time: Instant = at.plusSeconds(index.toLong()),
        received: Instant = time.plusSeconds(60), state: ReadingState = ReadingState.VALID,
        segment: SegmentRef = this.segment, operation: Int = index): Reading = Reading(
        ReadingRef(org, segment, id(200 + index)), MeterValue.of(segment.meter.kind, value),
        TimeDeclaration(time, "UTC", 0), received, member, device, ReadingOrigin.STANDALONE, ReadingMethod.TYPED,
        OperationRef(org, id(100 + operation)), classification = state)
    private fun opening(segment: SegmentRef = this.segment) = reading(0, 0, at, segment = segment)
    private fun fact(target: Reading, index: Int, operation: Int = index, reason: String = "Synthetic correction") = VoidFact(
        segment, id(300 + index), OperationRef(org, id(100 + operation)), target.reference, member, device, at.plusSeconds(3600), reason)
    private fun supersession(target: Reading, replacement: Reading, index: Int) = SupersessionFact(segment, id(300 + index),
        replacement.operation, target.reference, replacement.reference, member, device, at.plusSeconds(3600), "Observed typing error")
    private fun failure(expected: MeterFailure, block: () -> Unit) {
        assertEquals(expected, assertThrows(MeterDomainException::class.java, block).failure)
    }

    @Test fun `typed references reject noncanonical identifiers and every foreign component`() {
        failure(MeterFailure.SCOPE) { MeterRef(other, asset, id(21), MeterKind.ODOMETER) }
        failure(MeterFailure.SCOPE) { SegmentRef(other, meter, id(31)) }
        failure(MeterFailure.SCOPE) { ReadingRef(other, segment, id(201)) }
        failure(MeterFailure.INVALID) { MembershipRef(org, "") }
        failure(MeterFailure.INVALID) { DeviceRef(org, "00000000-0000-0000-0000-000000000000") }
        failure(MeterFailure.INVALID) { OperationRef(org, id(101).uppercase()) }
        failure(MeterFailure.INVALID) { PhotoRef(org, "personal-not-a-business-reference") }
        failure(MeterFailure.INVALID) { MeterRef(org, asset, id(21), MeterKind.ODOMETER, 0) }
    }

    @Test fun `reading validation retains source metadata without inferring origin method photo policy`() {
        for (origin in ReadingOrigin.entries) for (method in ReadingMethod.entries) {
            val value = Reading(ReadingRef(org, segment, id(201)), MeterValue.of(MeterKind.ODOMETER, 10), TimeDeclaration(at, "UTC", 3600),
                at.plusSeconds(60), member, device, origin, method, OperationRef(org, id(101)), classification = ReadingState.VALID)
            assertEquals(origin, value.origin); assertEquals(method, value.method)
            assertNull(value.photo); assertEquals(3600L, value.declared.deviceOffsetSeconds)
        }
        failure(MeterFailure.SCOPE) { Reading(ReadingRef(org, segment, id(201)), MeterValue.of(MeterKind.ODOMETER, 10),
            TimeDeclaration(at, "UTC", 0), at, MembershipRef(other, id(41)), device, ReadingOrigin.STANDALONE, ReadingMethod.TYPED, OperationRef(org, id(101)), classification = ReadingState.VALID) }
        failure(MeterFailure.INVALID) { Reading(ReadingRef(org, segment, id(201)), MeterValue.of(MeterKind.HOUR_METER, 10),
            TimeDeclaration(at, "UTC", 0), at, member, device, ReadingOrigin.STANDALONE, ReadingMethod.TYPED, OperationRef(org, id(101)), classification = ReadingState.VALID) }
    }

    @Test fun `invalid time and terminal initial classification cannot become observed evidence`() {
        for (declaration in listOf(TimeDeclaration(at, "Not/A_Zone", 0), TimeDeclaration(at, "UTC", 64801),
            TimeDeclaration(Instant.parse("+10000-01-01T00:00:00Z"), "UTC", 0))) {
            failure(MeterFailure.INVALID) { Reading(ReadingRef(org, segment, id(201)), MeterValue.of(MeterKind.ODOMETER, 10),
                declaration, at, member, device, ReadingOrigin.STANDALONE, ReadingMethod.TYPED, OperationRef(org, id(101)), classification = ReadingState.VALID) }
        }
        failure(MeterFailure.INVALID) { reading(1, 10, received = Instant.parse("0001-01-01T00:00:00Z")) }
        failure(MeterFailure.INVALID) { reading(1, 10, state = ReadingState.VOIDED) }
    }

    @Test fun `one real observed opening and independent kinds make in service facts`() {
        val distance = MeterSeries.open(segment, opening())
        assertEquals(0L, distance.latest(at)!!.value.units)
        val hours = MeterRef(org, asset, id(22), MeterKind.HOUR_METER)
        assertEquals(2, MeterOpenSet.validate(org, listOf(meter, hours)).size)
        failure(MeterFailure.METER_ALREADY_IN_SERVICE) { MeterOpenSet.validate(org, listOf(meter, MeterRef(org, asset, id(23), MeterKind.ODOMETER))) }
        failure(MeterFailure.INVALID) { MeterOpenSet.validate(org, listOf(meter, meter)) }
        failure(MeterFailure.SCOPE) { MeterOpenSet.validate(other, listOf(meter)) }
        val foreign = SegmentRef(org, MeterRef(org, asset, id(24), MeterKind.ODOMETER), id(32))
        failure(MeterFailure.SCOPE) { MeterSeries.open(segment, opening(foreign)) }
    }

    @Test fun `ordering and latest remain invariant for every arrival permutation and timestamp tie`() {
        val a = reading(1, 10, at.plusSeconds(100), at.plusSeconds(201))
        val b = reading(2, 20, at.plusSeconds(100), at.plusSeconds(200))
        val c = reading(3, 30, at.plusSeconds(100), at.plusSeconds(201))
        val suspect = reading(4, 999, at.plusSeconds(101), state = ReadingState.SUSPECT)
        for (permutation in permutations(listOf(a, b, c, suspect))) {
            val series = permutation.fold(MeterSeries.open(segment, opening())) { current, next -> current.append(next) }
            assertEquals(listOf(opening().reference.id, b.reference.id, a.reference.id, c.reference.id, suspect.reference.id), series.ordered().map { it.reference.id })
            assertEquals(c.reference.id, series.latest(at.plusSeconds(1000))!!.reference.id)
        }
    }

    @Test fun `equal observations from different operations remain while operation replay is rejected per meter`() {
        val first = reading(1, 10)
        val series = MeterSeries.open(segment, opening()).append(first).append(reading(2, 10))
        assertEquals(3, series.readings.size)
        failure(MeterFailure.OPERATION_ID_REUSED) { series.append(reading(3, 10, operation = 1)) }
        failure(MeterFailure.INVALID) { series.append(first) }
        val hours = SegmentRef(org, MeterRef(org, asset, id(22), MeterKind.HOUR_METER), id(32))
        assertEquals(1, MeterSeries.open(hours, opening(hours)).readings.size)
    }

    @Test fun `void is append only terminal and never removes original observations`() {
        val first = reading(1, 10)
        val original = MeterSeries.open(segment, opening()).append(first)
        val changed = original.void(fact(first, 10))
        assertEquals(ReadingState.VALID, first.classification)
        assertEquals(ReadingState.VALID, original.state(first.reference))
        assertEquals(ReadingState.VOIDED, changed.state(first.reference))
        assertEquals(2, changed.readings.size); assertEquals(1, changed.facts.size)
        assertEquals(opening().reference.id, changed.latest(at.plusSeconds(100))!!.reference.id)
        failure(MeterFailure.READING_ALREADY_SUPERSEDED) { changed.void(fact(first, 11)) }
        val replacement = reading(12, 11)
        failure(MeterFailure.READING_ALREADY_SUPERSEDED) { changed.supersede(supersession(first, replacement, 12), replacement) }
        assertEquals(1, changed.facts.size)
    }

    @Test fun `one correction operation appends its fact and replacement and a chain preserves originals`() {
        val wrong = reading(1, 148_190_000, state = ReadingState.SUSPECT)
        val replacement = reading(2, 184_190_000)
        val original = MeterSeries.open(segment, opening()).append(wrong)
        val corrected = original.supersede(supersession(wrong, replacement, 10), replacement)
        assertEquals(2, original.readings.size); assertEquals(3, corrected.readings.size)
        assertEquals(ReadingState.SUSPECT, wrong.classification)
        assertEquals(ReadingState.VOIDED, corrected.state(wrong.reference))
        assertEquals(replacement.reference.id, corrected.latest(at.plusSeconds(100))!!.reference.id)
        failure(MeterFailure.READING_ALREADY_SUPERSEDED) { corrected.supersede(supersession(wrong, reading(3, 1), 11), reading(3, 1)) }
        val next = reading(4, 184_191_000)
        val chain = corrected.supersede(supersession(replacement, next, 12), next)
        assertEquals(ReadingState.VOIDED, chain.state(replacement.reference))
        assertEquals(next.reference.id, chain.latest(at.plusSeconds(100))!!.reference.id)
        failure(MeterFailure.OPERATION_ID_REUSED) { chain.append(reading(5, 184_191_000, operation = 2)) }
    }

    @Test fun `missing anchors are insufficient while known equal endpoints retain a real zero anchor`() {
        val series = MeterSeries.open(segment, opening()).append(reading(1, 10))
        val missing = series.usage(at.minusSeconds(1), at.plusSeconds(10))
        assertEquals(UsageStatus.INSUFFICIENT_DATA, missing.status)
        assertEquals("NO_READING_BEFORE", missing.reason); assertNull(missing.value); assertNull(missing.from); assertEquals(series.latest(at.plusSeconds(10))!!.reference, missing.to!!.reference)
        val known = series.usage(at, at)
        assertEquals(0L, known.value!!.units)
        assertNotNull(known.from); assertEquals(known.from!!.reference.id, known.to!!.reference.id)
        failure(MeterFailure.INVALID) { series.usage(at.plusSeconds(1), at) }
        val decreasing = MeterSeries.open(segment, reading(0, 10, at)).append(reading(1, 9))
        failure(MeterFailure.INVALID) { decreasing.usage(at, at.plusSeconds(10)) }
    }

    @Test fun `all returned collections are defensive and diagnostic surfaces redact evidence`() {
        val first = reading(1, 10)
        val series = MeterSeries.open(segment, opening()).append(first).void(fact(first, 10))
        (series.readings as MutableList).clear()
        try { (series.facts as MutableList).clear() } catch (_: UnsupportedOperationException) { /* Immutable copies are equally safe. */ }
        assertEquals(2, series.readings.size); assertEquals(1, series.facts.size)
        for (value in listOf(meter, segment, first.reference, first, first.author, first.device, first.operation,
            PhotoRef(org, id(61)), series, series.facts.single(), series.usage(at, at))) {
            val text = value.toString()
            assertFalse(text.contains(org)); assertFalse(text.contains(first.reference.id)); assertFalse(text.contains("Synthetic correction"))
            assertTrue(text.contains("redacted"))
        }
    }

    @Test fun `open set snapshots a changing caller list before validating its owned facts`() {
        val foreignAsset = AssetRef(other, AssetId.parse(id(12)))
        val foreign = MeterRef(other, foreignAsset, id(22), MeterKind.HOUR_METER)
        var reads = 0
        val caller = object : AbstractList<MeterRef>() {
            override val size: Int get() = 1
            override fun get(index: Int): MeterRef {
                check(index == 0)
                return if (++reads <= 3) meter else foreign
            }
        }
        val result = MeterOpenSet.validate(org, caller)
        assertTrue(result.all { it.organizationId == org })
        assertEquals(listOf(meter), result)
    }

    private fun <T> permutations(values: List<T>): List<List<T>> = if (values.isEmpty()) listOf(emptyList()) else values.flatMapIndexed { index, value ->
        permutations(values.filterIndexed { at, _ -> at != index }).map { listOf(value) + it }
    }
}
