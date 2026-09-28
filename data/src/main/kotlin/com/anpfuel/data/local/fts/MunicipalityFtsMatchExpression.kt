package com.anpfuel.data.local.fts

/**
 * Builds FTS5 MATCH expressions from user input (UC-004).
 *
 * Tokens are uppercased to align with ANP municipality names and prefixed for partial search.
 */
object MunicipalityFtsMatchExpression {

    private val TOKEN_SPLIT_REGEX = Regex("\\s+")

    fun fromUserQuery(query: String): String {
        val tokens = query.trim()
            .replace("\"", "")
            .replace("'", "")
            .replace("*", "")
            .split(TOKEN_SPLIT_REGEX)
            .filter { it.isNotBlank() }
            .map { it.uppercase() }

        if (tokens.isEmpty()) {
            return ""
        }

        return tokens.joinToString(" ") { token -> "$token*" }
    }
}
