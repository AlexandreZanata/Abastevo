package com.anpfuel.domain.repository

import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.MunicipalityCatalogEntry
import com.anpfuel.domain.valueobject.MunicipalityLocationKey
import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * Port for IBGE municipality catalog queries and ANP data availability (UC-003, UC-004, UC-005).
 */
interface MunicipalityCatalogRepository {

    suspend fun getCatalogStates(): List<BrazilianState>

    suspend fun getCatalogMunicipalities(state: BrazilianState): List<MunicipalityCatalogEntry>

    suspend fun findCatalogEntry(
        state: BrazilianState,
        municipality: String,
    ): MunicipalityCatalogEntry?

    suspend fun resolveDataAvailability(
        state: BrazilianState,
        municipality: String,
        surveyWeek: SurveyWeek,
    ): DataAvailability

    suspend fun getLocationKeysWithDataForWeek(surveyWeek: SurveyWeek): Set<MunicipalityLocationKey>

    suspend fun getLocationKeysEverInAnp(): Set<MunicipalityLocationKey>

    /**
     * Operational warning for the survey week when published by ANP (Phase 12 enrichment).
     * Returns null until week catalog metadata is available.
     */
    suspend fun getOperationalNote(surveyWeek: SurveyWeek): String? = null
}
