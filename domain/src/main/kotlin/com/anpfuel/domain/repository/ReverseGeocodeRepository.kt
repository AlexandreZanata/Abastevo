package com.anpfuel.domain.repository

import com.anpfuel.domain.model.ReverseGeocodeResult
import com.anpfuel.domain.valueobject.DeviceLocation

sealed interface ReverseGeocodeOutcome {
    data class Success(val result: ReverseGeocodeResult) : ReverseGeocodeOutcome
    data object RateLimited : ReverseGeocodeOutcome
    data object MunicipalityNotInCatalog : ReverseGeocodeOutcome
    data object NetworkError : ReverseGeocodeOutcome
    data object InvalidResponse : ReverseGeocodeOutcome
}

/**
 * Port for Nominatim reverse geocoding with client-side cache (UC-012, BR-021).
 */
interface ReverseGeocodeRepository {

    suspend fun reverseGeocode(location: DeviceLocation): ReverseGeocodeOutcome
}
