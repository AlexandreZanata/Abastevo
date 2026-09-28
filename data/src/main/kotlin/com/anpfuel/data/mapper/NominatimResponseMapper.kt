package com.anpfuel.data.mapper

import com.anpfuel.domain.valueobject.BrazilianState
import org.json.JSONObject

/**
 * Maps Nominatim jsonv2 reverse responses to Brazilian state and municipality labels.
 */
object NominatimResponseMapper {

    data class ParsedAddress(
        val state: BrazilianState,
        val municipality: String,
        val displayName: String?,
    )

    fun parse(responseBody: String): ParsedAddress? {
        val root = runCatching { JSONObject(responseBody) }.getOrNull() ?: return null
        val address = root.optJSONObject("address") ?: return null
        val countryCode = address.optString("country_code").lowercase()
        if (countryCode != "br") {
            return null
        }

        val stateLabel = address.optString("state").takeIf { it.isNotBlank() }
            ?: address.optString("ISO3166-2-lvl4").substringAfterLast('-').takeIf { it.isNotBlank() }
            ?: return null
        val state = if (stateLabel.length == 2) {
            BrazilianState.fromAbbreviation(stateLabel)
        } else {
            AnpStateMapper.toBrazilianState(stateLabel)
        } ?: return null

        val municipality = sequenceOf("city", "town", "municipality", "village", "hamlet")
            .mapNotNull { key -> address.optString(key).takeIf { it.isNotBlank() } }
            .firstOrNull()
            ?: return null

        val displayName = root.optString("display_name").takeIf { it.isNotBlank() }

        return ParsedAddress(
            state = state,
            municipality = municipality,
            displayName = displayName,
        )
    }
}
