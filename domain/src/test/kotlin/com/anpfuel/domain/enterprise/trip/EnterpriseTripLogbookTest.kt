package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** EMP-TRIP-003 values that the shared logbook-v1 oracle does not print. */
class EnterpriseTripLogbookTest {
    private val trip = TripFixture.start(TripFixture.startFields(emptyMap()))
    private val org = TripFixture.alias("A")
    private val input = TripLogbookInput(TripLogbookRef(org, "5e000000-0000-4000-8000-000000000081"), MembershipRef(org, TripFixture.alias("M")),
        "NOTE", "Painel apagado", TimeDeclaration(EnterpriseTime.parseInstant("2026-11-14T13:00:00Z"), "America/Cuiaba", -14400))

    @Test fun vocabularyIsClosedAndOnlyNoteRequiresText() {
        assertEquals(13, TripLogbookType.entries.size)
        assertEquals(listOf("NOTE", "STOP", "ARRIVAL", "DEPARTURE"), TripLogbookType.entries.filter { it.supported }.map { it.name })
        TripLogbookType.entries.forEach { assertEquals(it == TripLogbookType.NOTE, it.requiresText) }
    }

    @Test fun logbookValuesNeverPrintPrivateContent() {
        val entry = trip.log(org, input)
        for (value in listOf<Any>(input, entry, input.reference!!)) {
            val text = value.toString()
            assertTrue(text.contains("redacted"), text)
            for (secret in listOf("5e000000", "Painel", "Cuiaba", "NOTE")) assertFalse(text.contains(secret), text)
        }
        assertEquals("Painel apagado", entry.text)
        assertEquals(TripState.IN_PROGRESS, trip.state)
    }
}
