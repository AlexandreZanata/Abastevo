package com.anpfuel.app.community

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

/**
 * CS08B RED → GREEN: TalkBack description carries counts plus choice
 * and review notice (B-BR-CS01 comprehension), never bare numbers.
 */
class CommunityHelpfulnessA11yTest {

    @Test
    fun `counts only when anonymous and clean`() {
        assertEquals(
            "3 helpful · 1 not helpful",
            helpfulnessTalkBackDescription("3 helpful · 1 not helpful", null, false),
        )
    }

    @Test
    fun `choice appended when present`() {
        assertEquals(
            "3 helpful · 1 not helpful, Your choice: HELPFUL",
            helpfulnessTalkBackDescription("3 helpful · 1 not helpful", "HELPFUL", false),
        )
    }

    @Test
    fun `review notice appended under review`() {
        assertEquals(
            "3 helpful · 1 not helpful, Your choice: HELPFUL, Counts under community review",
            helpfulnessTalkBackDescription("3 helpful · 1 not helpful", "HELPFUL", true),
        )
    }

    @Test
    fun `blank choice ignored`() {
        assertEquals(
            "0 helpful · 0 not helpful",
            helpfulnessTalkBackDescription("0 helpful · 0 not helpful", "  ", false),
        )
    }
}
