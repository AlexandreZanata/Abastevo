package com.anpfuel.data.local.fts

import com.anpfuel.domain.rule.MunicipalitySearchTextNormalizer

/**
 * Builds FTS MATCH expressions from user input (UC-004).
 *
 * Tokens are normalized up front (uppercase, diacritics stripped) because the
 * FTS4 simple tokenizer only folds ASCII case: raw accented queries would
 * match inconsistently depending on the case the user typed. Accent-aware
 * retrieval is completed by the bounded normalized-prefix supplement pool
 * (MEX-05), which covers the folded forms the FTS index cannot see.
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
            .map { MunicipalitySearchTextNormalizer.normalize(it) }
            .filter { it.isNotBlank() }

        if (tokens.isEmpty()) {
            return ""
        }

        return tokens.joinToString(" ") { token -> "$token*" }
    }
}
