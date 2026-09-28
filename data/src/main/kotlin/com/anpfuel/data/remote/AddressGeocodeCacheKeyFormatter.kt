package com.anpfuel.data.remote

/**
 * Normalizes a geocoding query into a DataStore-safe cache key so that
 * repeated requests for the same address never hit Nominatim twice (BR-021).
 */
object AddressGeocodeCacheKeyFormatter {

    private val WHITESPACE_REGEX = Regex("\\s+")

    fun format(query: String): String =
        query.trim()
            .lowercase()
            .replace(WHITESPACE_REGEX, " ")

    /**
     * Key for a structured request; the prefix guarantees it can never
     * collide with a free-text query of the same words.
     */
    fun formatStructured(street: String, city: String, state: String): String =
        "structured|${format(street)}|${format(city)}|${format(state)}"
}
