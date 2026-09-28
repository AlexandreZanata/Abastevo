package com.anpfuel.data.remote

/**
 * Parses optional ANP week-block operational warnings (Phase 12.2.3).
 */
internal object AnpListingOperationalNoteParser {

    private val OPERATIONAL_NOTE_PREFIX = Regex(
        """^\s*(NOTA|Aviso)\s*:?\s*(.+)$""",
        setOf(RegexOption.IGNORE_CASE, RegexOption.DOT_MATCHES_ALL),
    )

    fun parseOperationalNote(text: String): String? {
        val trimmed = text.trim()
        if (trimmed.isBlank()) {
            return null
        }

        val match = OPERATIONAL_NOTE_PREFIX.matchEntire(trimmed) ?: return null
        val body = match.groupValues[2].trim()
        return body.takeIf { it.isNotBlank() }
    }
}
