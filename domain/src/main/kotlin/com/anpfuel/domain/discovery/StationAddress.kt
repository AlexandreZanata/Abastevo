package com.anpfuel.domain.discovery

/** Source of the displayed address, independent of map point quality. */
enum class StationAddressSource(val wire: String) {
    ANP("anp"), RFB("rfb"), OSM("osm"), CNEFE("cnefe"), MUNICIPALITY("municipality");

    companion object {
        fun fromWire(wire: String?): StationAddressSource? = entries.firstOrNull { it.wire == wire }
    }
}

private val coordinateLabels = Regex("""\b(?:lat(?:itude)?|lon(?:gitude)?|lng|gps|geo)\b""", RegexOption.IGNORE_CASE)
private val coordinatePair = Regex("""[-+]?\d{1,3}(?:[.,]\d+)?\s*[,;/]\s*[-+]?\d{1,3}(?:[.,]\d+)?""")
private val municipalityCodeLabel = Regex("""\d{7}(?:\s*/\s*[a-z]{2})?""", RegexOption.IGNORE_CASE)
private val notAnAddress = setOf("NULL", "N/A", "NAO INFORMADO", "NÃO INFORMADO", "SEM ENDERECO", "SEM ENDEREÇO", "SN", "S/N")
private val states = setOf("AC", "AL", "AM", "AP", "BA", "CE", "DF", "ES", "GO", "MA", "MG", "MS", "MT", "PA", "PB", "PE", "PI", "PR", "RJ", "RN", "RO", "RR", "RS", "SC", "SE", "SP", "TO")
private val nameSpacing = Regex("""[\s\p{Z}]+""")

internal fun normalizedStationName(name: String): String = nameSpacing.replace(name, " ").trim()

/**
 * B-BR-STATION-NAME-01 — registry names often read "Trade (Legal)". When the
 * parenthesized part is the station's legal name, which the UI already shows
 * as the secondary line, the title keeps only the trade name. Any other
 * parenthesis, or a title that would become empty, is kept as is.
 */
fun stationTitle(displayName: String, legalName: String?): String {
    val legal = legalName?.let(::comparableStationName)?.takeIf { it.isNotEmpty() } ?: return displayName
    val match = trailingParenthesis.matchEntire(displayName.trim()) ?: return displayName
    val (outer, inner) = match.destructured
    return if (outer.isNotBlank() && comparableStationName(inner) == legal) normalizedStationName(outer) else displayName
}

private val trailingParenthesis = Regex("^(.*\\S)\\s*\\(([^()]+)\\)$")

/** Letters and digits only, lowercased: "Ltda." and "LTDA" compare equal. */
private fun comparableStationName(name: String): String =
    name.lowercase().filter(Char::isLetterOrDigit)

/** A single known name is shown once, regardless of case or source spacing. */
fun secondaryStationLegalName(displayName: String, legalName: String?): String? =
    legalName?.trim()?.takeIf { it.isNotEmpty() &&
        !normalizedStationName(it).equals(normalizedStationName(displayName), ignoreCase = true) }

internal fun isCoordinateLiteral(value: String): Boolean {
    val withoutLabels = coordinateLabels.replace(value, "")
    return coordinatePair.matches(value) ||
        (withoutLabels != value && withoutLabels.any(Char::isDigit) && withoutLabels.none(Char::isLetter))
}

private fun textComponent(value: String): Boolean =
    value.any(Char::isLetter) && !value.any(Char::isISOControl) && !isCoordinateLiteral(value) &&
        !municipalityCodeLabel.matches(value) && value.uppercase() !in notAnAddress

private fun Map<String, String>.component(vararg keys: String, usable: (String) -> Boolean): String? =
    keys.firstNotNullOfOrNull { key -> this[key]?.trim()?.takeIf(usable) }

/** A bounded set of textual address components; never reads map coordinates. */
fun formatStationAddress(address: Map<String, String>, stationState: String? = null): String {
    val city = address.component("municipio", "municipality_name", "municipality", "city", usable = ::textComponent)
    val uf = sequenceOf(address["uf"], stationState).filterNotNull()
        .map { it.trim().uppercase() }.firstOrNull { it in states }
    val locality = city?.let { if (uf == null) it else "$it/$uf" }
    val street = address.component("street", "logradouro", usable = ::textComponent)
    val number = address.component("number", "numero") {
        it.isNotBlank() && !it.any(Char::isISOControl) && !isCoordinateLiteral(it) && !it.equals("null", ignoreCase = true)
    }
    val line = if (street != null) listOfNotNull(street, number).joinToString(", ") else
        address.component("raw", "endereco", usable = ::textComponent)
    if (line == null) return locality.orEmpty()
    val district = address.component("district", "bairro", usable = ::textComponent)
    val area = listOfNotNull(district, locality).joinToString(", ")
    return if (area.isEmpty()) line else "$line – $area"
}
