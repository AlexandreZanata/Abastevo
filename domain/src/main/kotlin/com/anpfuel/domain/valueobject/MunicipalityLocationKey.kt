package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException

/**
 * Stable identity for a municipality within a federative unit.
 * Used for catalog matching and ANP data availability checks.
 */
data class MunicipalityLocationKey(
    val state: BrazilianState,
    val municipality: String,
) {
    init {
        if (municipality.isBlank()) {
            throw DomainException("Municipality name must not be blank")
        }
    }

    companion object {
        fun from(entry: MunicipalityCatalogEntry): MunicipalityLocationKey =
            MunicipalityLocationKey(
                state = entry.state,
                municipality = entry.municipality,
            )
    }
}
