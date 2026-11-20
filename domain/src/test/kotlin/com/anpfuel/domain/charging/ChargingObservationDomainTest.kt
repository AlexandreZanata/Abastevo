package com.anpfuel.domain.charging

import java.time.format.DateTimeParseException
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class ChargingObservationDomainTest {
    private val id = "00000000-0000-4000-8000-000000000001"
    private val at = "2026-01-02T12:00:00Z"
    private val event = ChargingOperationalEvent(ChargingOperationalStatus.WORKED, ChargingTargetScope.SITE, null, at)
    private val draft = ChargingObservationDraft(id, ChargingConnector.UNKNOWN, 1500, ChargingCondition.STANDARD, "2026-01-02", "Checked")

    @Test
    fun `dated event applies only to a canonical target`() {
        assertEquals(event, draft.copy(operational = event).operational)
        assertEquals(id, event.copy(targetScope = ChargingTargetScope.CONNECTOR, connectionGroupId = id).connectionGroupId)
        assertThrows<IllegalArgumentException> { event.copy(connectionGroupId = id) }
        assertThrows<IllegalArgumentException> { event.copy(targetScope = ChargingTargetScope.EQUIPMENT) }
        assertThrows<IllegalArgumentException> { event.copy(targetScope = ChargingTargetScope.EQUIPMENT, connectionGroupId = "bad") }
        assertThrows<DateTimeParseException> { event.copy(observedAt = "yesterday") }
    }

    @Test
    fun `observation rejects malformed dates places prices and notes`() {
        assertNull(draft.copy(placeId = null, priceMilliBrlPerKwh = null).priceMilliBrlPerKwh)
        assertThrows<IllegalArgumentException> { draft.copy(placeId = "bad") }
        assertThrows<IllegalArgumentException> { draft.copy(placeId = null, operational = event) }
        assertThrows<IllegalArgumentException> { draft.copy(priceMilliBrlPerKwh = 0) }
        assertThrows<IllegalArgumentException> { draft.copy(priceMilliBrlPerKwh = 10_000_001) }
        assertThrows<IllegalArgumentException> { draft.copy(observedDay = "01/02/2026") }
        assertThrows<DateTimeParseException> { draft.copy(observedDay = "2026-02-30") }
        assertThrows<IllegalArgumentException> { draft.copy(note = "x".repeat(1025)) }
        assertThrows<IllegalArgumentException> { draft.copy(note = "line\nbreak") }
    }
}
