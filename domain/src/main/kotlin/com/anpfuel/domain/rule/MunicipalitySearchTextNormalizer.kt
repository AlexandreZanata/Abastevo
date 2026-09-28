package com.anpfuel.domain.rule

import java.text.Normalizer

/**
 * Accent-insensitive normalization for municipality FTS ranking (BR-017).
 */
object MunicipalitySearchTextNormalizer {

    private val DIACRITIC_MARKS_REGEX = Regex("\\p{Mn}+")
    private val WHITESPACE_REGEX = Regex("\\s+")

    fun normalize(text: String): String =
        Normalizer.normalize(text.trim(), Normalizer.Form.NFD)
            .replace(DIACRITIC_MARKS_REGEX, "")
            .replace("'", "")
            .replace("-", " ")
            .replace(WHITESPACE_REGEX, " ")
            .uppercase()
}
