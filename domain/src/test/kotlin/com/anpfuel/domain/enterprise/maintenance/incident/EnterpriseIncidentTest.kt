package com.anpfuel.domain.enterprise.maintenance.incident

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class EnterpriseIncidentTest {
    private fun failure(block: () -> Unit) = assertThrows(IncidentDomainException::class.java) { block() }.failure

    @Test fun `the transition table is exactly the state model`() {
        val want = mapOf(
            IncidentState.REPORTED to IncidentAction.entries.toSet(),
            IncidentState.ACKNOWLEDGED to setOf(IncidentAction.TREAT, IncidentAction.RESOLVE, IncidentAction.DISMISS),
            IncidentState.IN_TREATMENT to setOf(IncidentAction.RESOLVE, IncidentAction.DISMISS),
        )
        for (state in IncidentState.entries) {
            for (action in IncidentAction.entries) {
                assertEquals(want[state].orEmpty().contains(action), Incident.allowed(state, action), "$state $action")
            }
            assertEquals(state == IncidentState.RESOLVED || state == IncidentState.DISMISSED, state.terminal)
        }
        assertFalse(IncidentAction.ACKNOWLEDGE.needsNote)
        assertTrue(IncidentAction.entries.filter { it != IncidentAction.ACKNOWLEDGE }.all { it.needsNote })
    }

    @Test fun `closed vocabularies`() {
        assertEquals(IncidentKind.FINE_NOTICE, IncidentKind.parse("FINE_NOTICE"))
        assertEquals(IncidentReportType.DAMAGE, IncidentReportType.parse("DAMAGE"))
        listOf("", "theft", " THEFT", "FLOOD").forEach { assertEquals(IncidentFailure.INVALID, failure { IncidentKind.parse(it) }) }
        listOf("", "OPEN", "acknowledge").forEach { assertEquals(IncidentFailure.INVALID, failure { IncidentAction.parse(it) }) }
        assertEquals(IncidentFailure.INVALID, failure { IncidentState.parse("CLOSED") })
    }

    @Test fun `report text is never part of a printed value`() {
        val report = IncidentReport.draft("INCIDENT", "ACCIDENT", "PRIVATE-DESCRIPTION-7731", "PRIVATE-LOCATION-7731", true)
        assertEquals("PRIVATE-DESCRIPTION-7731", report.description)
        assertFalse(report.toString().contains("PRIVATE"))
        val command = IncidentCommand("RESOLVE", 1, "PRIVATE-NOTE-7731", null, listOf("9c000000-0000-4000-8000-000000005002"))
        assertFalse(command.toString().contains("PRIVATE"))
    }

    @Test fun `the text profile matches the SQL profile`() {
        mapOf("a" to true, " a " to true, "\u200bx" to true, "" to false, " " to false, "\u3000" to false, "\u00a0" to false, "\u2028" to false,
            "a\nb" to false, "a\tb" to false, "a\u0085" to false, "a\u007f" to false, "a\uD800" to false).forEach { (raw, want) ->
            assertEquals(want, Incident.validText(raw, 10), raw)
        }
        assertFalse(Incident.validText("a".repeat(11), 10))
        assertTrue(Incident.validText("ã".repeat(10), 10))
    }
}
