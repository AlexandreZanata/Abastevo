package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.ReportingZones
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fleet.AssetId
import com.anpfuel.domain.enterprise.fleet.AssetRef
import com.anpfuel.domain.enterprise.meter.MeterDomainException
import com.anpfuel.domain.enterprise.meter.MeterKind
import com.anpfuel.domain.enterprise.meter.MeterPresence
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

class EnterpriseTripTest {
    private fun id(n: Int) = "5e000000-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val org = id(1)
    private val other = id(2)
    private val asset = AssetRef(org, AssetId.parse(id(21)))

    private fun reading(readingN: Int, segmentN: Int = 61, meterN: Int = 51, kind: MeterKind = MeterKind.ODOMETER, modulus: Long? = null) =
        ReadingRef(org, SegmentRef(org, MeterRef(org, asset, id(meterN), kind, modulus), id(segmentN)), id(readingN))
    private fun observed(ref: ReadingRef, units: Long) =
        TripMeterEvidence.Observed(ref, MeterValue.of(ref.segment.meter.kind, units))
    private fun at(raw: String) = TimeDeclaration(EnterpriseTime.parseInstant(raw), "America/Cuiaba", -14400)
    private fun startInput() = TripStartInput(TripRef(org, id(11)), MembershipRef(org, id(41)), asset, EmployeeRef(org, id(31)),
        observed(reading(71), 184_320_000), at("2026-11-14T12:00:00Z"), ReportingZones("America/Cuiaba"))
    private fun finishInput(author: MembershipRef = MembershipRef(org, id(41))) =
        TripFinishInput(author, observed(reading(72), 184_610_000), at("2026-11-14T15:00:00Z"))
    private fun failure(block: () -> Unit): TripDomainException = assertThrows(TripDomainException::class.java) { block() }

    @Test fun `closed vocabularies have no defaults`() {
        assertEquals(setOf(TripState.COMPLETED, TripState.CANCELLED), TripState.entries.filter { it.terminal }.toSet())
        for (raw in listOf("", "unknown", " IN_PROGRESS", "APPROVED", "DRAFT")) {
            assertEquals(TripFailure.INVALID, failure { TripState.parse(raw) }.failure)
        }
        for (raw in listOf("", "APPROVE", "start", "SUBMIT")) assertEquals(TripFailure.INVALID, failure { TripCommand.parse(raw) }.failure)
        for (raw in listOf("", "COMPLETED", "ACCEPTED", "approved")) assertEquals(TripFailure.INVALID, failure { TripReviewState.parse(raw) }.failure)
        TripState.entries.forEach { assertSame(it, TripState.parse(it.name)) }
        TripCommand.entries.forEach { assertSame(it, TripCommand.parse(it.name)) }
        TripReviewState.entries.forEach { assertSame(it, TripReviewState.parse(it.name)) }
    }

    @Test fun `the transition table is total and only finishes an in-progress trip`() {
        for (state in TripState.entries) for (command in TripCommand.entries) {
            val expected = when {
                state.terminal -> TripFailure.INVALID_TRANSITION
                command in setOf(TripCommand.PAUSE, TripCommand.RESUME, TripCommand.CANCEL) -> TripFailure.UNSUPPORTED_COMMAND
                state != TripState.IN_PROGRESS -> TripFailure.UNSUPPORTED_COMMAND
                command == TripCommand.START -> TripFailure.INVALID_TRANSITION
                else -> null
            }
            if (expected == null) assertEquals(TripState.COMPLETED, TripLifecycle.transition(state, command))
            else assertEquals(expected, failure { TripLifecycle.transition(state, command) }.failure, "$state $command")
        }
    }

    @Test fun `standing separates completion from approval`() {
        for (state in TripState.entries) for (review in TripReviewState.entries) {
            val standing = TripStanding(state, review)
            assertEquals(state == TripState.COMPLETED, standing.completed)
            assertEquals(review == TripReviewState.APPROVED, standing.approved)
        }
    }

    @Test fun `the VAL-01 trip records exactly 290 000 m and is not approved`() {
        val started = Trip.start(org, startInput())
        assertEquals(TripState.IN_PROGRESS, started.state)
        assertEquals(TripUsageStatus.NOT_MEASURED, started.meterUsage.status)
        assertEquals(TripUsageReason.NOT_FINISHED, started.meterUsage.reason)
        assertNull(started.meterUsage.delta)
        val done = started.finish(org, finishInput())
        assertEquals(TripState.COMPLETED, done.state)
        assertEquals(TripUsageStatus.MEASURED, done.meterUsage.status)
        assertEquals(290_000L, done.meterUsage.delta)
        assertEquals(MeterKind.ODOMETER, done.meterUsage.kind)
        assertNull(done.meterUsage.reason)
        assertEquals(started.reference, done.reference)
        assertEquals(started.asset, done.asset)
        assertEquals(started.driver, done.driver)
        assertEquals(started.reporting, done.reporting)
        assertSame(started.started, done.started)
        assertNull(started.finished)
        assertEquals(TripState.IN_PROGRESS, started.state)
        assertFalse(TripStanding(done.state, TripReviewState.DRAFT).approved)
        assertEquals(TripFailure.INVALID_TRANSITION, failure { done.finish(org, finishInput()) }.failure)
    }

    @Test fun `only the starting member finishes and only in its organization`() {
        val started = Trip.start(org, startInput())
        assertEquals(TripFailure.FORBIDDEN, failure { started.finish(org, finishInput(MembershipRef(org, id(42)))) }.failure)
        assertEquals(TripFailure.SCOPE, failure { started.finish(other, finishInput()) }.failure)
        assertEquals(TripFailure.SCOPE, failure { started.finish("not-a-uuid", finishInput()) }.failure)
        assertEquals(TripFailure.SCOPE, failure { started.finish(org, finishInput(MembershipRef(other, id(41)))) }.failure)
        assertEquals(TripFailure.INVALID, failure { started.finish(org, TripFinishInput()) }.failure)
    }

    @Test fun `absent context is listed once in order`() {
        val error = failure { Trip.start(org, startInput().copy(asset = null, driver = null, departure = null, declared = null)) }
        assertEquals(TripFailure.INCOMPLETE, error.failure)
        assertEquals(listOf(TripMissing.ASSET, TripMissing.DRIVER, TripMissing.DEPARTURE_EVIDENCE, TripMissing.DECLARATION), error.missing)
        val source = mutableListOf(TripMissing.ASSET)
        val copied = TripDomainException(TripFailure.INCOMPLETE, source)
        source += TripMissing.DRIVER
        assertEquals(listOf(TripMissing.ASSET), copied.missing)
        assertEquals(emptyList<TripMissing>(), TripDomainException().missing)
        assertEquals("trip: incomplete", error.message)
        assertEquals(TripFailure.INVALID, failure { Trip.start(org, TripStartInput()) }.failure)
    }

    @Test fun `evidence is exact and of the meter kind`() {
        assertThrows(MeterDomainException::class.java) { MeterPresence.missing(" ") }
        assertEquals(TripFailure.INVALID, failure {
            TripMeterEvidence.Observed(reading(71), MeterValue.of(MeterKind.HOUR_METER, 1))
        }.failure)
        assertEquals(TripFailure.INVALID, failure {
            TripMeterEvidence.Observed(reading(71, modulus = 1000), MeterValue.of(MeterKind.ODOMETER, 1000))
        }.failure)
        val kept = TripMeterEvidence.Absent(MeterPresence.missing("Painel apagado"))
        assertEquals("Painel apagado", kept.missing.reason)
    }

    @Test fun `references are scoped and typed`() {
        assertEquals(TripFailure.SCOPE, failure { TripRef("not-a-uuid", id(11)) }.failure)
        assertEquals(TripFailure.INVALID, failure { EmployeeRef(org, "bad") }.failure)
        assertEquals(TripFailure.INVALID, failure { MembershipRef(org, id(41).uppercase()) }.failure)
        assertEquals(TripRef(org, id(11)), TripRef(org, id(11)))
        assertNotEquals(TripRef(org, id(11)) as Any, EmployeeRef(org, id(11)) as Any)
        assertNotEquals(TripRef(org, id(11)), TripRef(other, id(11)))
        assertEquals(TripRef(org, id(11)).hashCode(), TripRef(org, id(11)).hashCode())
        assertNotEquals(TripRef(org, id(11)), Any())
    }

    @Test fun `private values print redacted`() {
        val started = Trip.start(org, startInput())
        val done = started.finish(org, finishInput())
        val values = listOf<Any>(startInput(), finishInput(), started, done, done.started, done.finished!!, done.started.evidence,
            TripMeterEvidence.Absent(MeterPresence.missing("Painel apagado")), done.meterUsage, TripRef(org, id(11)))
        for (value in values) {
            val text = value.toString()
            assertTrue(text.contains("redacted"), text)
            for (secret in listOf("5e000000", "184320000", "290000", "Painel", "Cuiaba")) assertFalse(text.contains(secret), text)
        }
    }
}
