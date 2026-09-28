package com.anpfuel.data.remote

import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * Parses pt-BR ANP listing week section headers (Phase 12.2.1).
 * Example: "07/06/2026 a 13/06/2026"
 */
internal object AnpListingWeekHeaderParser {

    private val WEEK_HEADER_PATTERN = Regex(
        """(\d{1,2}/\d{1,2}/\d{4})\s+a\s+(\d{1,2}/\d{1,2}/\d{4})""",
        RegexOption.IGNORE_CASE,
    )

    fun parseWeekHeader(text: String): SurveyWeek? {
        val match = WEEK_HEADER_PATTERN.find(text.trim()) ?: return null
        return try {
            val startDate = AnpBrazilianDateParser.parseDate(match.groupValues[1]) ?: return null
            val endDate = AnpBrazilianDateParser.parseDate(match.groupValues[2]) ?: return null
            SurveyWeek(startDate = startDate, endDate = endDate)
        } catch (_: Exception) {
            null
        }
    }
}
