package com.anpfuel.data.repository

import com.anpfuel.data.local.catalog.MunicipalityCatalogSeeder
import com.anpfuel.data.local.dao.AveragePriceDao
import com.anpfuel.data.local.dao.MunicipalityCatalogDao
import com.anpfuel.data.mapper.MunicipalityCatalogMapper
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.rule.MunicipalityDataAvailabilityRule
import com.anpfuel.domain.rule.MunicipalitySearchTextNormalizer
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.MunicipalityCatalogEntry
import com.anpfuel.domain.valueobject.MunicipalityLocationKey
import com.anpfuel.domain.valueobject.SurveyWeek
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class MunicipalityCatalogRepositoryImpl @Inject constructor(
    private val municipalityCatalogDao: MunicipalityCatalogDao,
    private val averagePriceDao: AveragePriceDao,
    private val catalogSeeder: MunicipalityCatalogSeeder,
) : MunicipalityCatalogRepository {

    override suspend fun getCatalogStates(): List<BrazilianState> {
        catalogSeeder.seedIfEmpty()
        return municipalityCatalogDao.findDistinctStates()
            .mapNotNull(BrazilianState::fromAbbreviation)
            .sortedBy { it.abbreviation }
    }

    override suspend fun getCatalogMunicipalities(state: BrazilianState): List<MunicipalityCatalogEntry> {
        catalogSeeder.seedIfEmpty()
        return municipalityCatalogDao.findByState(state.abbreviation)
            .mapNotNull(MunicipalityCatalogMapper::toDomain)
    }

    override suspend fun findCatalogEntry(
        state: BrazilianState,
        municipality: String,
    ): MunicipalityCatalogEntry? {
        catalogSeeder.seedIfEmpty()
        val normalized = MunicipalitySearchTextNormalizer.normalize(municipality)
        return municipalityCatalogDao.findByStateAndNormalizedName(state.abbreviation, normalized)
            ?.let(MunicipalityCatalogMapper::toDomain)
    }

    override suspend fun resolveDataAvailability(
        state: BrazilianState,
        municipality: String,
        surveyWeek: SurveyWeek,
    ): DataAvailability {
        val entry = findCatalogEntry(state, municipality)
            ?: return DataAvailability.NEVER_IN_ANP

        return MunicipalityDataAvailabilityRule.resolve(
            entry = entry,
            surveyWeekId = DomainId.forSurveyWeek(surveyWeek),
            municipalitiesWithDataThisWeek = getLocationKeysWithDataForWeek(surveyWeek),
            municipalitiesEverInAnp = getLocationKeysEverInAnp(),
        )
    }

    override suspend fun getLocationKeysWithDataForWeek(surveyWeek: SurveyWeek): Set<MunicipalityLocationKey> {
        val surveyWeekId = DomainId.forSurveyWeek(surveyWeek).value
        return averagePriceDao.findDistinctLocationsBySurveyWeek(surveyWeekId)
            .mapNotNull { row -> resolveCatalogLocationKey(row.state, row.municipality) }
            .toSet()
    }

    override suspend fun getLocationKeysEverInAnp(): Set<MunicipalityLocationKey> {
        catalogSeeder.seedIfEmpty()
        return averagePriceDao.findDistinctLocationsEverImported()
            .mapNotNull { row -> resolveCatalogLocationKey(row.state, row.municipality) }
            .toSet()
    }

    private suspend fun resolveCatalogLocationKey(
        stateAbbrev: String,
        municipality: String,
    ): MunicipalityLocationKey? {
        val state = BrazilianState.fromAbbreviation(stateAbbrev) ?: return null
        return findCatalogEntry(state, municipality)?.locationKey
    }
}
