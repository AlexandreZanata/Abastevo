package com.anpfuel.data.remote

import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class AnpListingWeekHeaderParserTest {

    @Test
    fun parsesGovBrWeekSectionHeader() {
        val week = AnpListingWeekHeaderParser.parseWeekHeader("07/06/2026 a 13/06/2026")

        assertEquals(SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13"), week)
    }

    @Test
    fun ignoresNonHeaderText() {
        assertNull(AnpListingWeekHeaderParser.parseWeekHeader("Preços médios semanais"))
    }

    @Test
    fun rejectsInvalidDateRange() {
        assertNull(AnpListingWeekHeaderParser.parseWeekHeader("32/13/2026 a 40/01/2026"))
    }
}
