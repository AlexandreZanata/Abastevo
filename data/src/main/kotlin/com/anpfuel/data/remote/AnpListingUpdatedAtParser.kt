package com.anpfuel.data.remote

import java.time.LocalDate

/**
 * Parses optional ANP link suffix "(Atualizado em d/M/yyyy)" (Phase 12.2.2).
 */
internal object AnpListingUpdatedAtParser {

    private val UPDATED_AT_PATTERN = Regex(
        """\(Atualizado em\s+(\d{1,2}/\d{1,2}/\d{4})\)""",
        RegexOption.IGNORE_CASE,
    )

    fun parseUpdatedAt(text: String): LocalDate? {
        val match = UPDATED_AT_PATTERN.find(text) ?: return null
        return AnpBrazilianDateParser.parseDate(match.groupValues[1])
    }
}
