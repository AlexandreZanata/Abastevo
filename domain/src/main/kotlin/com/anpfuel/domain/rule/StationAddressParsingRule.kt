package com.anpfuel.domain.rule

import com.anpfuel.domain.model.RetailStation

/**
 * Result of decomposing a raw ANP station address into searchable parts.
 */
data class ParsedStationAddress(
    /** Street name with common type abbreviations expanded, e.g. "AVENIDA AMAZONAS". */
    val street: String,
    /** Street number, or null when the address is s/n or a highway address. */
    val number: String?,
    /** Neighborhood (bairro), when present. */
    val neighborhood: String?,
)

/**
 * Splits ANP station addresses (typically "STREET, NUMBER, NEIGHBORHOOD") into a
 * structured form suitable for Nominatim's structured geocoding parameters.
 *
 * Handles common variants: missing neighborhood, missing number, "S/N" markers,
 * and leading street-type abbreviations (AV. -> AVENIDA, R. -> RUA, ROD. -> RODOVIA, ...).
 */
object StationAddressParsingRule {

    private val STREET_TYPE_EXPANSIONS = mapOf(
        "AV" to "AVENIDA",
        "R" to "RUA",
        "ROD" to "RODOVIA",
        "EST" to "ESTRADA",
        "AL" to "ALAMEDA",
        "TV" to "TRAVESSA",
        "TRV" to "TRAVESSA",
        "PCA" to "PRACA",
        "PC" to "PRACA",
        "Q" to "QUADRA",
        "QD" to "QUADRA",
        "LOT" to "LOTE",
        "LT" to "LOTE",
        "BR" to "RODOVIA",
    )

    private val NO_NUMBER_TOKENS = setOf("S/N", "SN", "S/NUMERO", "SEM NUMERO", "S-N")

    private val HIGHWAY_PREFIXES = listOf("RODOVIA ", "BR ", "BR-")

    private val NUMBER_REGEX = Regex("""^\d+(?:[ -.][A-Z0-9]+)?$""")

    fun parse(station: RetailStation): ParsedStationAddress {
        val normalized = StationAddressNormalizationRule.normalizeAddress(station.address)
        val segments = normalized
            .split(',')
            .map { it.trim().trimEnd('.') }
            .filter { it.isNotEmpty() }

        val rawStreet = segments.firstOrNull().orEmpty()
        val street = expandStreetType(rawStreet)

        val rest = segments.drop(1)
        val number = rest.firstOrNull()?.takeIf { isNumberToken(it) }
        val neighborhood = rest
            .drop(if (number != null) 1 else 0)
            .joinToString(", ")
            .takeIf { it.isNotEmpty() }

        return ParsedStationAddress(street = street, number = number, neighborhood = neighborhood)
    }

    private fun isNumberToken(token: String): Boolean {
        val upper = token.uppercase().trim()
        return NUMBER_REGEX.matches(upper) || NO_NUMBER_TOKENS.any { upper == it }
    }

    /**
     * Expands a leading street-type abbreviation ("AV. BRASIL" -> "AVENIDA BRASIL").
     * For short token sequences like "RODOVIA GO 020" both the generic type and any
     * highway marker are expanded, but trailing abbreviations are left untouched.
     */
    fun expandStreetType(rawStreet: String): String {
        val tokens = rawStreet
            .split(' ')
            .map { it.trim() }
            .filter { it.isNotEmpty() }
            .toMutableList()
        if (tokens.isEmpty()) return rawStreet

        var i = 0
        while (i < tokens.size && i < 2) {
            val key = tokens[i].trimEnd('.')
            val replacement = STREET_TYPE_EXPANSIONS[key]
            if (replacement != null) {
                // Expand at most the first token, plus a second one for
                // compounds like "ROD. BR-116" -> "RODOVIA RODOVIA-116".
                if (replacement == "RODOVIA" || i == 0) {
                    tokens[i] = replacement
                    i++
                    continue
                }
            }
            break
        }
        return tokens.joinToString(" ")
    }

    fun isHighwayAddress(parsed: ParsedStationAddress): Boolean {
        val upper = parsed.street.uppercase()
        return HIGHWAY_PREFIXES.any { upper.startsWith(it) }
    }
}
