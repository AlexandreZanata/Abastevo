package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertSame
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** EMP-TRIP-012 behaviour the shared handover-v1 rows cannot express (B-BR-EMP-394/395). */
class EnterpriseTripHandoverTest {
    private val org = TripFixture.alias("A")
    private fun id(index: Int) = "5e000000-0000-4000-8000-" + index.toString().padStart(12, '0')
    private fun started(): Trip = TripFixture.start(TripFixture.startFields(mapOf("org" to "A")))
    private fun input(trip: Trip, reason: String? = "Troca de turno") = TripHandoverInput(MembershipRef(org, id(43)), MembershipRef(org, id(42)),
        EmployeeRef(org, id(32)), trip.revision, reason, EnterpriseTime.parseInstant("2026-11-14T13:00:00Z"))

    @Test fun `a handover keeps the trip, its history and its input unchanged`() {
        val trip = started()
        val handed = trip.handover(org, input(trip))
        assertEquals(2L, handed.revision)
        assertEquals(TripState.IN_PROGRESS, handed.state)
        assertSame(trip.started, handed.started)
        assertEquals(trip.driver, handed.driver)
        assertEquals(MembershipRef(org, id(42)), handed.responsible)
        assertEquals(trip.started.author, handed.responsibleAt(EnterpriseTime.parseInstant("2026-11-14T12:59:59.999999999Z")))
        assertEquals(1L, trip.revision)
        assertTrue(trip.handovers.isEmpty())
        val paused = handed.pause(org, TripLifecycleInput(handed.responsible, 2, null, TimeDeclaration(EnterpriseTime.parseInstant("2026-11-14T13:30:00Z"),
            "America/Cuiaba", -14400)))
        assertEquals(3L, paused.revision)
        assertEquals(1, paused.handovers.size)
        val missing = assertThrows(TripDomainException::class.java) { trip.handover(org, TripHandoverInput()) }
        assertEquals(TripFailure.INVALID, missing.failure)
    }

    @Test fun `handover inputs and facts print redacted`() {
        val trip = started()
        val handed = trip.handover(org, input(trip, "Painel quebrado"))
        for (value in listOf(input(trip, "Painel quebrado"), handed.handovers.single(), handed)) {
            val text = value.toString()
            assertTrue("redacted" in text && "Painel" !in text && "5e000000" !in text, text)
        }
    }
}
