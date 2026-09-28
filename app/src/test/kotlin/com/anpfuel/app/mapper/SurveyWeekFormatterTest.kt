package com.anpfuel.app.mapper

import com.anpfuel.domain.valueobject.SurveyWeek
import java.util.Locale
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class SurveyWeekFormatterTest {

    @Test
    fun formatRangeUsesPortugueseConnectorForPtLocale() {
        val week = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")

        val formatted = SurveyWeekFormatter.formatRange(week, Locale.forLanguageTag("pt-BR"))

        assertTrue(formatted.contains(" a "))
    }

    @Test
    fun formatRangeUsesEnDashForEnglishLocale() {
        val week = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")

        val formatted = SurveyWeekFormatter.formatRange(week, Locale.US)

        assertTrue(formatted.contains("–"))
    }

    @Test
    fun formatRangeCompactUsesShortPatternForSameMonthInEnglish() {
        val week = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")

        val formatted = SurveyWeekFormatter.formatRangeCompact(week, Locale.US)

        assertEquals("Jun 7–13, 2026", formatted)
    }

    @Test
    fun formatRangeCompactUsesShortPatternForSameMonthInPortuguese() {
        val week = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")

        val formatted = SurveyWeekFormatter.formatRangeCompact(week, Locale.forLanguageTag("pt-BR"))

        assertTrue(formatted.contains("7–13"))
        assertTrue(formatted.contains("2026"))
        assertTrue(!formatted.contains(" a "))
    }

    @Test
    fun formatRangeCompactHandlesCrossMonthWeekInPortuguese() {
        val week = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")

        val formatted = SurveyWeekFormatter.formatRangeCompact(week, Locale.forLanguageTag("pt-BR"))

        assertTrue(formatted.contains("–"))
        assertTrue(formatted.contains("2026"))
    }
}
