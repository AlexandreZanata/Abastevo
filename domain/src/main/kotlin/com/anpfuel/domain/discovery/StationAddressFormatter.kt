package com.anpfuel.domain.discovery

import java.util.Locale

/**
 * Human-readable station address from verbatim source components
 * ("Avenida Brasil, 120 · Centro"). Codes (IBGE, postal code) and the raw
 * duplicate never reach the user; coordinates are never an address.
 * Source text is uppercase, so words are title-cased for display only.
 */
object StationAddressFormatter {

    private val streetKeys = listOf("street", "logradouro")
    private val numberKeys = listOf("number", "numero")
    private val districtKeys = listOf("district", "bairro")
    private val cityKeys = listOf("municipio", "city")
    private val connectors = setOf("de", "da", "do", "das", "dos", "e", "em", "na", "no")
    private val roman = Regex("^(I|II|III|IV|V|VI|VII|VIII|IX|X|XI|XII)$")
    private val coordinateLabels = Regex("""\b(?:lat(?:itude)?|lon(?:gitude)?|lng|gps|geo)\b""", RegexOption.IGNORE_CASE)
    private val coordinatePair = Regex("""[-+]?\d{1,3}(?:[.,]\d+)?\s*[,;/]\s*[-+]?\d{1,3}(?:[.,]\d+)?""")
    private val municipalityCode = Regex("""\d{7}(?:\s*/\s*[a-z]{2})?""", RegexOption.IGNORE_CASE)
    private val ptBr = Locale.forLanguageTag("pt-BR")

    fun format(address: Map<String, String>): String {
        fun first(keys: List<String>) = keys.firstNotNullOfOrNull { key -> address[key]?.let(::clean)?.takeIf { it.isNotEmpty() } }
        val rawParts = address["raw"]?.trim()?.takeIf(::readableText).orEmpty().split(',').map { it.trim() }.filter { it.isNotEmpty() }
        val street = first(streetKeys)?.takeIf(::readableText) ?: rawParts.firstOrNull()?.takeIf(::readableText)
        val number = first(numberKeys)?.takeUnless(::coordinateLiteral)
            ?: rawParts.getOrNull(1)?.takeIf { part -> part.any(Char::isDigit) && !coordinateLiteral(part) }
        val district = first(districtKeys)?.takeIf(::readableText)
        if (street == null) {
            val city = first(cityKeys)?.takeIf(::readableText) ?: return ""
            return listOfNotNull(titleCase(city), address["uf"]?.trim()?.takeIf { it.isNotEmpty() }?.uppercase(ptBr))
                .joinToString(" · ")
        }
        val line = if (number != null && !street.contains(number)) "${titleCase(street)}, $number" else titleCase(street)
        return listOfNotNull(line, district?.let(::titleCase)).joinToString(" · ")
    }

    private fun readableText(value: String): Boolean =
        value.any(Char::isLetter) && !coordinateLiteral(value) && !municipalityCode.matches(value)

    private fun coordinateLiteral(value: String): Boolean {
        val withoutLabels = coordinateLabels.replace(value, "")
        return coordinatePair.matches(value) ||
            (withoutLabels != value && withoutLabels.any(Char::isDigit) && withoutLabels.none(Char::isLetter))
    }

    /** Source fields may end with separators ("AVENIDA X ,"). */
    private fun clean(value: String): String = value.trim().trimEnd(',', ';', '.', '-', ' ').trim()

    private fun titleCase(text: String): String =
        text.trim().split(Regex("\\s+")).mapIndexed { index, word ->
            val lower = word.lowercase(ptBr)
            when {
                index > 0 && lower in connectors -> lower
                else -> word.split('-').joinToString("-", transform = ::titleWord)
            }
        }.joinToString(" ")

    private fun titleWord(part: String): String {
        val upper = part.uppercase(ptBr)
        return when {
            roman.matches(upper) -> upper
            part.any(Char::isDigit) -> part
            // Short vowel-less tokens are acronyms (JK, BR), not words.
            part.length in 2..3 && upper.none { it in "AEIOUÁÉÍÓÚÂÊÔÃÕ" } -> upper
            else -> part.lowercase(ptBr).replaceFirstChar { it.titlecase(ptBr) }
        }
    }
}
