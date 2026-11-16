package com.anpfuel.domain.enterprise.tracking

import com.anpfuel.domain.enterprise.trip.MembershipRef
import com.anpfuel.domain.enterprise.trip.TripRef
import com.anpfuel.domain.enterprise.trip.TripState
import java.io.File
import java.time.Duration
import java.time.Instant
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class EnterpriseTrackingSessionTest {
    private val org = TrackingFixture.id(1)
    private val driver = MembershipRef(org, TrackingFixture.id(31))
    private val trip = TrackingTripFacts(TripRef(org, TrackingFixture.id(21)), driver, TripState.IN_PROGRESS)
    private fun at(minute: Long): Instant = TrackingFixture.base.plus(Duration.ofMinutes(minute))
    private fun open(): TrackingSession = TrackingSession.open(org, TrackingOpenInput(TrackingSessionRef(org, TrackingFixture.id(11)), trip, driver,
        TrackingOrigin.TRIP_SCREEN, TrackingPolicy.EXPECTED, at(0)))
    private fun action(permission: LocationPermission, minute: Long) =
        TrackingDriverAction(trip, driver, TrackingOrigin.TRIP_SCREEN, TrackingDevice(permission, true), at(minute))
    private fun failure(block: () -> Unit): TrackingFailure = assertThrows<TrackingDomainException> { block() }.failure

    @Test fun `a tracked trip pauses, resumes only on the trip screen and ends with the trip`() {
        var session = open()
        assertEquals(TrackingSessionState.NOT_STARTED, session.state)
        assertEquals(TrackingCause.NOT_STARTED, session.cause)
        assertNull(session.collecting(org, trip))
        session = session.start(org, action(LocationPermission.PRECISE, 10))
        assertEquals(LocationPermission.PRECISE, session.collecting(org, trip))
        val paused = session.follow(org, trip.copy(state = TripState.PAUSED), at(20))
        assertEquals(TrackingSessionState.SUSPENDED, paused.state)
        val tripResumed = paused.follow(org, trip, at(30))
        assertEquals(paused.history.size, tripResumed.history.size, "resuming the trip resumed recording by itself")
        val resumed = paused.resume(org, action(LocationPermission.APPROXIMATE, 30))
        val done = resumed.follow(org, trip.copy(state = TripState.COMPLETED), at(90))
        assertEquals(TrackingSessionState.ENDED, done.state)
        assertTrue(done.coarse)
        assertNull(done.precision)
        assertEquals(listOf(LocationPermission.PRECISE, LocationPermission.APPROXIMATE), done.recordingPeriods.map { it.precision })
        assertEquals(at(90), done.recordingPeriods.last().end)
        assertEquals(TrackingActor.TRIP, done.history.last().actor)
        val quality = TrackQualityRules.classify(TrackEvidence(20400, 1020, 2, done.coarse))
        assertEquals(TrackQuality.APPROXIMATE, quality.quality)
        assertEquals(950L, quality.coveragePermille)
        assertEquals(TrackingFailure.INVALID_TRANSITION, failure { done.resume(org, action(LocationPermission.PRECISE, 95)) })
    }

    @Test fun `absent inputs are invalid and never assumed`() {
        assertEquals(TrackingFailure.INVALID, failure { TrackingSession.open(org, TrackingOpenInput()) })
        val session = open()
        assertEquals(TrackingFailure.INVALID, failure { session.start(org, TrackingDriverAction()) })
        assertEquals(TrackingFailure.INVALID, failure { session.start(org, action(LocationPermission.PRECISE, 10).copy(device = TrackingDevice())) })
        assertEquals(TrackingFailure.INVALID, failure { session.end(org, TrackingEvent()) })
        assertEquals(TrackingFailure.INVALID, failure { session.follow(org, TrackingTripFacts(), at(10)) })
        assertEquals(TrackingFailure.INVALID, failure { session.follow(org, trip, null) })
        assertEquals(TrackingFailure.SCOPE, failure { session.collecting("not-an-organization", trip) })
        assertEquals(TrackingFailure.SCOPE, failure { TrackingSessionRef("not-an-organization", TrackingFixture.id(11)) })
        assertEquals(TrackingFailure.INVALID, failure { TrackingSessionRef(org, TrackingFixture.id(11).uppercase()) })
        assertEquals(TrackingFailure.INVALID, failure { TrackQualityRules.classify(TrackEvidence(TrackQualityRules.MAX_EXPECTED_SECONDS + 1, 0, 2, false)) })
        assertEquals(TrackingFailure.INVALID, failure {
            TrackingSession.open(org, TrackingOpenInput(TrackingSessionRef(org, TrackingFixture.id(11)), trip, driver, TrackingOrigin.TRIP_SCREEN,
                TrackingPolicy.EXPECTED, Instant.parse("+10000-01-01T00:00:00Z")))
        })
    }

    @Test fun `references compare by organization and identity`() {
        val a = TrackingSessionRef(org, TrackingFixture.id(11))
        assertEquals(a, TrackingSessionRef(org, TrackingFixture.id(11)))
        assertEquals(a.hashCode(), TrackingSessionRef(org, TrackingFixture.id(11)).hashCode())
        assertNotEquals(a, TrackingSessionRef(TrackingFixture.id(2), TrackingFixture.id(11)))
        assertNotEquals(a, TrackingSessionRef(org, TrackingFixture.id(12)))
        assertFalse(a.equals(TrackingFixture.id(11)))
        assertTrue(TrackingSessionState.ENDED.terminal)
        assertFalse(TrackingSessionState.RECORDING.terminal)
    }

    @Test fun `the trip never depends on tracking`() {
        val tripSources = File("src/main/kotlin/com/anpfuel/domain/enterprise/trip").listFiles().orEmpty().filter { it.extension == "kt" }
        assertTrue(tripSources.isNotEmpty())
        tripSources.forEach { assertFalse(it.readText().contains("enterprise.tracking"), "the trip depends on tracking: ${it.name}") }
    }

    @Test fun `private values print redacted`() {
        val session = open().start(org, action(LocationPermission.PRECISE, 10))
        val values = listOf<Any>(session, session.reference, session.history.last(), session.recordingPeriods.first(), trip,
            action(LocationPermission.PRECISE, 10), TrackingEvent(trip, TrackingCause.TRIP_PAUSED, driver, at(20)),
            TrackingDevice(LocationPermission.PRECISE, true), TrackingOpenInput(session.reference, trip, driver))
        for (value in values) {
            val text = value.toString()
            assertTrue(text.contains("redacted"), text)
            for (secret in listOf("9b000000", "2026", "PRECISE", "RECORDING", "TRIP_PAUSED")) assertFalse(text.contains(secret), text)
        }
    }
}
