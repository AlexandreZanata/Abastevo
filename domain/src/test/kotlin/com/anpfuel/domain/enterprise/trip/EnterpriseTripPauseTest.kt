package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertSame
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** EMP-TRIP-011 behaviour the shared trip-v2 rows cannot express (B-BR-EMP-390). */
class EnterpriseTripPauseTest {
    private val org = TripFixture.alias("A")
    private fun at(raw: String) = TimeDeclaration(EnterpriseTime.parseInstant(raw), "America/Cuiaba", -14400)
    private fun started(): Trip = TripFixture.start(TripFixture.startFields(mapOf("org" to "A")))
    private fun input(trip: Trip, expected: Long, raw: String, reason: String? = null) = TripLifecycleInput(trip.responsible, expected, reason, at(raw))
    private fun event(trip: Trip, raw: String) =
        trip.log(org, TripLogbookInput(TripLogbookRef(org, TripFixture.alias("5e000000-0000-4000-8000-000000000081")), trip.responsible, "STOP", null, at(raw)))

    @Test fun `cancellation closes the logbook and a paused trip still takes events`() {
        val trip = started()
        assertEquals(1L, trip.revision)
        assertSame(trip.started.author, trip.responsible)
        val paused = trip.pause(org, input(trip, 1, "2026-11-14T12:30:00Z", "Almoço"))
        event(paused, "2026-11-14T12:40:00Z")
        val cancelled = paused.cancel(org, input(paused, 2, "2026-11-14T13:00:00Z", "Cliente cancelou"))
        event(cancelled, "2026-11-14T13:00:00Z")
        val after = assertThrows(TripLogbookException::class.java) { event(cancelled, "2026-11-14T13:00:00.000000001Z") }
        assertEquals(TripLogbookFailure.EVENT_AFTER_FINISH, after.failure)
        val fact = requireNotNull(cancelled.cancellation)
        assertEquals(listOf(TripCommand.PAUSE, TripCommand.CANCEL), cancelled.lifecycle.map { it.command })
        assertEquals(3L, fact.revision)
        assertEquals(TripState.PAUSED, fact.from)
        assertEquals("Cliente cancelou", fact.reason)
        assertNull(paused.cancellation)
        assertNull(cancelled.finished)
        assertFalse(cancelled.crossing.finished)
        assertEquals(TripUsageReason.CANCELLED, cancelled.meterUsage.reason)
        assertEquals(TripUsageStatus.NOT_MEASURED, cancelled.meterUsage.status)
    }

    @Test fun `a missing author is invalid and inputs print redacted`() {
        val trip = started()
        val failure = assertThrows(TripDomainException::class.java) { trip.pause(org, TripLifecycleInput(null, 1, null, at("2026-11-14T13:00:00Z"))) }
        assertEquals(TripFailure.INVALID, failure.failure)
        val defaulted = assertThrows(TripDomainException::class.java) { trip.cancel(org, TripLifecycleInput()) }
        assertEquals(TripFailure.INVALID, defaulted.failure)
        val cancelled = trip.cancel(org, input(trip, 1, "2026-11-14T13:00:00Z", "Painel apagado"))
        for (value in listOf(input(trip, 1, "2026-11-14T13:00:00Z", "Painel apagado"), requireNotNull(cancelled.cancellation), cancelled)) {
            val text = value.toString()
            assertTrue("redacted" in text && "Painel" !in text && "5e000000" !in text, text)
        }
    }
}
