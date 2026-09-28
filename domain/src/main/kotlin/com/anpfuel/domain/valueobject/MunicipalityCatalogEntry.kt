package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException

/**
 * A municipality in the national IBGE catalog, optionally enriched with ANP aliases.
 * Availability for a survey week is derived at query time (BR-010, BR-016).
 */
data class MunicipalityCatalogEntry(
    val state: BrazilianState,
    val municipality: String,
    val ibgeCode: String? = null,
) {
    init {
        if (municipality.isBlank()) {
            throw DomainException("Municipality name must not be blank")
        }
        ibgeCode?.let { code ->
            if (!IBGE_CODE_PATTERN.matches(code)) {
                throw DomainException("IBGE code must be exactly 7 digits")
            }
        }
    }

    val locationKey: MunicipalityLocationKey
        get() = MunicipalityLocationKey(state = state, municipality = municipality)

    /**
     * Whether this municipality has imported average prices for [surveyWeekId].
     * [municipalitiesWithDataForWeek] must already be scoped to that week.
     */
    fun hasAnpDataForWeek(
        surveyWeekId: DomainId,
        municipalitiesWithDataForWeek: Set<MunicipalityLocationKey>,
    ): Boolean {
        require(surveyWeekId.value.isNotBlank()) {
            "surveyWeekId must not be blank"
        }
        return locationKey in municipalitiesWithDataForWeek
    }

    companion object {
        private val IBGE_CODE_PATTERN = Regex("^\\d{7}$")
    }
}
