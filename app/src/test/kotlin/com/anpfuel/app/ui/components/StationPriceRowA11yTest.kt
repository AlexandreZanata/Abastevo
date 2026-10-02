package com.anpfuel.app.ui.components

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

// P24-T02 RED → GREEN: TalkBack description must carry price + recency
// (B-BR-C01 comprehension), not only the navigate verb.
class StationPriceRowA11yTest {

    @Test
    fun `price only when no recency`() {
        assertEquals(
            "Posto Centro, preço R$ 5,79",
            stationRowTalkBackDescription("Posto Centro, preço R$ 5,79", null),
        )
    }

    @Test
    fun `blank recency is ignored`() {
        assertEquals(
            "Posto Centro, preço R$ 5,79",
            stationRowTalkBackDescription("Posto Centro, preço R$ 5,79", "  "),
        )
    }

    @Test
    fun `recency is appended when present`() {
        assertEquals(
            "Posto Centro, preço R$ 5,79, Coleta: 10 jun 2026",
            stationRowTalkBackDescription(
                "Posto Centro, preço R$ 5,79",
                "Coleta: 10 jun 2026",
            ),
        )
    }
}
