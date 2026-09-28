package com.anpfuel.data.local.preferences

import com.anpfuel.domain.model.ReverseGeocodeResult
import com.anpfuel.domain.valueobject.BrazilianState

/**
 * Abstraction for geocode cache (enables JVM tests without DataStore).
 */
interface GeocodeCacheStore {

    suspend fun get(cacheKey: String): ReverseGeocodeResult?

    suspend fun put(cacheKey: String, result: ReverseGeocodeResult)
}

internal object GeocodeCacheCodec {

    private const val FIELD_SEPARATOR = "|"

    fun encode(result: ReverseGeocodeResult): String =
        listOf(
            result.state.abbreviation,
            result.municipality,
            result.displayName.orEmpty(),
        ).joinToString(FIELD_SEPARATOR)

    fun decode(raw: String): ReverseGeocodeResult? {
        val parts = raw.split(FIELD_SEPARATOR, limit = 3)
        if (parts.size < 2) {
            return null
        }
        val state = BrazilianState.fromAbbreviation(parts[0]) ?: return null
        val municipality = parts[1]
        val displayName = parts.getOrNull(2)?.takeIf { it.isNotBlank() }
        return ReverseGeocodeResult(
            state = state,
            municipality = municipality,
            displayName = displayName,
        )
    }
}
