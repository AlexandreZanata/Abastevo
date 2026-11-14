package com.anpfuel.domain.enterprise.meter

import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fleet.AssetId
import com.anpfuel.domain.enterprise.fleet.AssetRef
import java.time.Instant
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseMeterCaptureTest {
    private fun id(index: Int) = "8c000000-0000-4000-8000-" + index.toString().padStart(12, '0')
    private val org = id(1)
    private val other = id(2)
    private val segment = SegmentRef(org, MeterRef(org, AssetRef(org, AssetId.parse(id(11))), id(21), MeterKind.ODOMETER), id(31))
    private val at = Instant.parse("2026-11-02T00:00:00Z")
    private val member = MembershipRef(org, id(41))
    private val device = DeviceRef(org, id(51))
    private val operation = OperationRef(org, id(101))
    private fun reading(device: DeviceRef? = this.device, capture: CaptureSource? = null,
        author: MembershipRef = member, operation: OperationRef = this.operation): Reading = Reading(
        ReadingRef(org, segment, id(201)), MeterValue.of(MeterKind.ODOMETER, 10), TimeDeclaration(at, "UTC", 0),
        at.plusSeconds(60), author, device, ReadingOrigin.STANDALONE, ReadingMethod.TYPED, operation,
        classification = ReadingState.VALID, capture = capture)
    private fun failure(expected: MeterFailure, work: () -> Unit) {
        assertEquals(expected, assertThrows(MeterDomainException::class.java, work).failure)
    }

    @Test fun `capture has exact tags scoped locators and redacted diagnostics`(): Unit {
        for (kind in CaptureKind.entries) {
            val source = CaptureSource(org, kind, id(51))
            assertEquals(org, source.organizationId); assertEquals(kind, source.kind); assertEquals(id(51), source.id)
            assertFalse(source.toString().contains(source.id)); assertFalse(source.toString().contains(org))
        }
        for (raw in listOf("", "DEVICE", "web_session", "WEB_SESSION ")) failure(MeterFailure.INVALID) { CaptureKind.parse(raw) }
        for (raw in listOf("", "00000000-0000-0000-0000-000000000000", "8C000000-0000-4000-8000-000000000051", "private-session-token")) {
            failure(MeterFailure.INVALID) { CaptureSource(org, CaptureKind.WEB_SESSION, raw) }
        }
        failure(MeterFailure.SCOPE) { CaptureSource("", CaptureKind.WEB_SESSION, id(51)) }
    }

    @Test fun `legacy device remains enrolled shorthand with canonical provenance`(): Unit {
        val original = reading()
        assertEquals(CaptureKind.ENROLLED_DEVICE, original.capture.kind)
        assertEquals(device.id, original.capture.id)
        assertEquals(device, original.device); assertEquals(device, original.deviceOrNull)
        assertEquals(org, original.capture.organizationId)
    }

    @Test fun `explicit sources reject ambiguity foreign evidence and fabricated web device`(): Unit {
        for (kind in CaptureKind.entries) {
            val source = CaptureSource(org, kind, id(51))
            val original = reading(device = null, capture = source)
            assertEquals(source, original.capture)
            if (kind == CaptureKind.ENROLLED_DEVICE) assertEquals(device, original.deviceOrNull)
            else { assertNull(original.deviceOrNull); failure(MeterFailure.INVALID) { original.device } }
            failure(MeterFailure.INVALID) { reading(capture = source) }
            failure(MeterFailure.INVALID) { reading(device = DeviceRef(org, id(52)), capture = source) }
            failure(MeterFailure.SCOPE) { reading(device = null, capture = CaptureSource(other, kind, id(51))) }
            failure(MeterFailure.SCOPE) { reading(device = null, capture = source, author = MembershipRef(other, id(41))) }
            failure(MeterFailure.SCOPE) { reading(device = null, capture = source, operation = OperationRef(other, id(101))) }
        }
        failure(MeterFailure.INVALID) { reading(device = null) }
    }
}
