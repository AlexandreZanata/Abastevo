package com.anpfuel.domain.repository

import com.anpfuel.domain.valueobject.GeoCoordinates

/**
 * A single geocoding lookup. Callers provide several of these in priority order
 * (most precise first); the repository tries each until one succeeds.
 */
sealed interface GeocodeRequest {

    val fallbackLabel: String

    /** Maps directly to Nominatim's structured search parameters. */
    data class Structured(
        val street: String,
        val city: String,
        val state: String,
        val country: String = "Brazil",
    ) : GeocodeRequest {
        override val fallbackLabel: String get() = "structured:$street"
    }

    /** Single free-text query. */
    data class FreeText(val query: String) : GeocodeRequest {
        override val fallbackLabel: String get() = "free-text"
    }
}

interface AddressGeocodeRepository {

    /** Single-attempt geocode for callers that only have a free-text query. */
    suspend fun geocode(query: String): AddressGeocodeOutcome

    /**
     * Tries each request in order, pausing between attempts to respect the
     * provider's rate limit. Returns the first success, or the last outcome
     * (other than a mid-cascade [AddressGeocodeOutcome.NotFound]) otherwise.
     */
    suspend fun geocode(requests: List<GeocodeRequest>): AddressGeocodeOutcome
}

sealed interface AddressGeocodeOutcome {

    data class Success(val coordinates: GeoCoordinates) : AddressGeocodeOutcome

    /** No results returned; the next fallback query should be attempted. */
    object NotFound : AddressGeocodeOutcome

    /** Transport-level failure after retries; do not keep hammering the provider. */
    object NetworkError : AddressGeocodeOutcome

    /** HTTP 429; provider asked us to slow down. */
    object RateLimited : AddressGeocodeOutcome
}
