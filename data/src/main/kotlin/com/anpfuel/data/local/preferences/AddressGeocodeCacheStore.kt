package com.anpfuel.data.local.preferences

import com.anpfuel.domain.valueobject.GeoCoordinates

/**
 * Abstraction for the forward geocode cache (enables JVM tests without DataStore).
 */
interface AddressGeocodeCacheStore {

    suspend fun get(cacheKey: String): GeoCoordinates?

    suspend fun put(cacheKey: String, coordinates: GeoCoordinates)
}

internal object AddressGeocodeCacheCodec {

    private const val FIELD_SEPARATOR = ","

    fun encode(coordinates: GeoCoordinates): String =
        "${coordinates.latitude}$FIELD_SEPARATOR${coordinates.longitude}"

    fun decode(raw: String): GeoCoordinates? {
        val parts = raw.split(FIELD_SEPARATOR, limit = 2)
        if (parts.size < 2) {
            return null
        }
        val latitude = parts[0].toDoubleOrNull() ?: return null
        val longitude = parts[1].toDoubleOrNull() ?: return null
        return runCatching { GeoCoordinates.of(latitude, longitude) }.getOrNull()
    }
}
