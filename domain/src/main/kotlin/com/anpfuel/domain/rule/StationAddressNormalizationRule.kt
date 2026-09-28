package com.anpfuel.domain.rule

import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.valueobject.BrazilianState

/**
 * BR-026 — Builds query strings for external map apps and geocoding services from ANP station data.
 */
object StationAddressNormalizationRule {

    private val WHITESPACE_REGEX = Regex("\\s+")
    private val CONTROL_CHARS_REGEX = Regex("\\p{C}+")

    private const val MIN_SUFFICIENT_ADDRESS_LENGTH = 5

    fun normalizeAddress(raw: String): String =
        raw.replace(CONTROL_CHARS_REGEX, "")
            .trim()
            .replace(WHITESPACE_REGEX, " ")

    fun buildNavigationQuery(
        station: RetailStation,
        preferredMunicipality: String? = null,
        preferredState: BrazilianState? = null,
    ): String {
        val municipality = station.municipality.ifBlank {
            preferredMunicipality?.trim().orEmpty()
        }
        val state = station.state
        val normalizedAddress = normalizeAddress(station.address)
        val displayName = station.displayName().trim()
        val queryBody = if (isAddressSufficient(normalizedAddress)) {
            prependStationNameIfMissing(
                displayName = displayName,
                query = appendLocationIfMissing(
                    address = normalizedAddress,
                    municipality = municipality,
                    stateLabel = state.abbreviation,
                ),
            )
        } else {
            "${displayName}, ${formatLocationSuffix(municipality, state.abbreviation)}"
        }

        return appendCountry(queryBody)
    }

    /**
     * UC-015 — Builds a Nominatim geocoding query from ANP station data.
     *
     * Unlike [buildNavigationQuery], the station trade name is never included: Nominatim
     * resolves street addresses, not business names, and a leading business name can make
     * the service return no results at all. Full state names are used because
     * abbreviations are not matched in street queries.
     */
    fun buildGeocodingQuery(
        station: RetailStation,
        preferredMunicipality: String? = null,
        preferredState: BrazilianState? = null,
    ): String {
        val municipality = station.municipality.ifBlank {
            preferredMunicipality?.trim().orEmpty()
        }
        val state = station.state
        val normalizedAddress = normalizeAddress(station.address)

        // Comma-separated parts match the format Nominatim resolves reliably
        // (verified against the live API): street, municipality, state, country.
        val addressIncludesMunicipality = municipality.isNotBlank() &&
            normalizedAddress.contains(municipality, ignoreCase = true)
        val parts = buildList {
            if (isAddressSufficient(normalizedAddress)) add(normalizedAddress)
            if (municipality.isNotBlank() && !addressIncludesMunicipality) add(municipality)
            add(state.displayName)
        }

        return appendCountry(parts.joinToString(", "))
    }

    internal fun prependStationNameIfMissing(displayName: String, query: String): String {
        if (displayName.isBlank()) {
            return query
        }
        if (query.contains(displayName, ignoreCase = true)) {
            return query
        }
        return "$displayName, $query"
    }

    internal fun isAddressSufficient(normalizedAddress: String): Boolean =
        normalizedAddress.length >= MIN_SUFFICIENT_ADDRESS_LENGTH

    private fun formatLocationSuffix(municipality: String, stateLabel: String): String =
        "$municipality - $stateLabel"

    private fun appendLocationIfMissing(
        address: String,
        municipality: String,
        stateLabel: String,
    ): String {
        val hasMunicipality = municipality.isNotBlank() &&
            address.contains(municipality, ignoreCase = true)
        val hasState = address.contains(stateLabel, ignoreCase = true)

        return when {
            hasMunicipality && hasState -> address
            hasMunicipality -> "$address, $stateLabel"
            else -> "$address, ${formatLocationSuffix(municipality, stateLabel)}"
        }
    }

    private fun appendCountry(query: String): String {
        val trimmed = query.trim()
        if (trimmed.endsWith("Brazil", ignoreCase = true) || trimmed.endsWith("Brasil", ignoreCase = true)) {
            return trimmed
        }
        return "$trimmed, Brazil"
    }
}
