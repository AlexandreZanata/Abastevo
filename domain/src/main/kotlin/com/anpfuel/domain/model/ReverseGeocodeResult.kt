package com.anpfuel.domain.model

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.valueobject.BrazilianState

/**
 * Resolved municipality from reverse geocoding (UC-012).
 */
data class ReverseGeocodeResult(
    val state: BrazilianState,
    val municipality: String,
    val displayName: String? = null,
) {
    init {
        if (municipality.isBlank()) {
            throw DomainException("ReverseGeocodeResult municipality must not be blank")
        }
    }
}
