package com.anpfuel.data.remote

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test
import java.time.LocalDate

class AnpBrazilianDateParserTest {

    @Test
    fun parsesBrazilianDateWithSingleDigitParts() {
        assertEquals(LocalDate.of(2026, 6, 12), AnpBrazilianDateParser.parseDate("12/6/2026"))
    }

    @Test
    fun parsesBrazilianDateWithZeroPaddedParts() {
        assertEquals(LocalDate.of(2026, 6, 7), AnpBrazilianDateParser.parseDate("07/06/2026"))
    }

    @Test
    fun returnsNullForInvalidCalendarDate() {
        assertNull(AnpBrazilianDateParser.parseDate("31/02/2026"))
    }
}
