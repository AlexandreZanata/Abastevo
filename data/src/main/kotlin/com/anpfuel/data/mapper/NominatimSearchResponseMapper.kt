package com.anpfuel.data.mapper

import com.anpfuel.domain.valueobject.GeoCoordinates
import org.json.JSONArray

/**
 * Maps Nominatim jsonv2 `/search` responses to [GeoCoordinates] (UC-015).
 */
object NominatimSearchResponseMapper {

    fun parse(responseBody: String): GeoCoordinates? {
        val results = runCatching { JSONArray(responseBody) }.getOrNull() ?: return null
        val firstResult = results.optJSONObject(FIRST_RESULT_INDEX) ?: return null

        val latitude = firstResult.optString("lat").toDoubleOrNull() ?: return null
        val longitude = firstResult.optString("lon").toDoubleOrNull() ?: return null

        return runCatching { GeoCoordinates.of(latitude, longitude) }.getOrNull()
    }

    private const val FIRST_RESULT_INDEX = 0
}
