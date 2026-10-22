package com.anpfuel.data.repository

import com.anpfuel.data.local.catalog.MunicipalityCatalogSeeder
import com.anpfuel.data.local.dao.AveragePriceDao
import com.anpfuel.data.local.dao.MunicipalityCatalogDao
import com.anpfuel.data.mapper.MunicipalityCatalogMapper
import com.anpfuel.domain.model.EnrichedSearchCandidate
import com.anpfuel.domain.model.MunicipalitySearchResult
import com.anpfuel.domain.model.SearchEnrichment
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.rule.MunicipalityDataAvailabilityRule
import com.anpfuel.domain.rule.MunicipalitySearchTextNormalizer
import com.anpfuel.domain.valueobject.AvailabilitySnapshot
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
        val snapshot = resolveAvailabilitySnapshot(surveyWeek)

        return MunicipalityDataAvailabilityRule.resolve(
            entry = entry,
            surveyWeekId = DomainId.forSurveyWeek(surveyWeek),
            municipalitiesWithDataThisWeek = snapshot.withDataThisWeek,
            municipalitiesEverInAnp = snapshot.everInAnp,
        )
    }

    override suspend fun getLocationKeysWithDataForWeek(surveyWeek: SurveyWeek): Set<MunicipalityLocationKey> =
        resolveAvailabilitySnapshot(surveyWeek).withDataThisWeek

    override suspend fun getLocationKeysEverInAnp(): Set<MunicipalityLocationKey> =
        resolveAvailabilitySnapshot(surveyWeek = null).everInAnp

    override suspend fun resolveAvailabilitySnapshot(surveyWeek: SurveyWeek?): AvailabilitySnapshot {
        catalogSeeder.seedIfEmpty()
        val rows = averagePriceDao.findLocationAvailability(surveyWeekIdOrEmpty(surveyWeek))
        if (rows.isEmpty()) {
            return AvailabilitySnapshot.empty()
        }
        val index = loadCatalogIndex(rows.map { row -> row.state }.toSet())
        return splitSnapshot(rows, index)
    }

    override suspend fun enrichSearchResults(
        results: List<MunicipalitySearchResult>,
        surveyWeek: SurveyWeek?,
    ): SearchEnrichment {
        if (results.isEmpty()) {
            return SearchEnrichment(
                candidates = emptyList(),
                snapshot = AvailabilitySnapshot.empty(),
            )
        }
        catalogSeeder.seedIfEmpty()
        val rows = averagePriceDao.findLocationAvailability(surveyWeekIdOrEmpty(surveyWeek))
        val states = results.map { result -> result.state.abbreviation }.toSet() +
            rows.map { row -> row.state }.toSet()
        val index = loadCatalogIndex(states)
        val candidates = results.map { result ->
            EnrichedSearchCandidate(
                result = result,
                entry = index[indexKey(result.state.abbreviation, result.municipality)],
            )
        }
        return SearchEnrichment(
            candidates = candidates,
            snapshot = splitSnapshot(rows, index),
        )
    }

    private fun surveyWeekIdOrEmpty(surveyWeek: SurveyWeek?): String =
        surveyWeek?.let { DomainId.forSurveyWeek(it).value } ?: ""

    /**
     * MEX-04 prepared catalog: one indexed state read per involved state,
     * keyed by (state, normalized_name) exactly like [findCatalogEntry].
     * Never a per-row or per-result query.
     */
    private suspend fun loadCatalogIndex(
        stateAbbreviations: Set<String>,
    ): Map<CatalogIndexKey, MunicipalityCatalogEntry> {
        val index = mutableMapOf<CatalogIndexKey, MunicipalityCatalogEntry>()
        for (abbreviation in stateAbbreviations.sorted()) {
            // Unknown abbreviations can never match a catalog entry; skip the read.
            if (BrazilianState.fromAbbreviation(abbreviation) == null) {
                continue
            }
            for (entity in municipalityCatalogDao.findByState(abbreviation)) {
                val entry = MunicipalityCatalogMapper.toDomain(entity) ?: continue
                index[CatalogIndexKey(abbreviation, entity.normalizedName)] = entry
            }
        }
        return index
    }

    private fun splitSnapshot(
        rows: List<com.anpfuel.data.local.model.MunicipalityLocationAvailabilityRow>,
        index: Map<CatalogIndexKey, MunicipalityCatalogEntry>,
    ): AvailabilitySnapshot {
        val withData = mutableSetOf<MunicipalityLocationKey>()
        val ever = mutableSetOf<MunicipalityLocationKey>()
        for (row in rows) {
            val key = resolveRowKey(row.state, row.municipality, index) ?: continue
            ever.add(key)
            if (row.hasWeekData) {
                withData.add(key)
            }
        }
        return AvailabilitySnapshot(
            withDataThisWeek = withData,
            everInAnp = ever,
        )
    }

    private fun resolveRowKey(
        stateAbbreviation: String,
        municipality: String,
        index: Map<CatalogIndexKey, MunicipalityCatalogEntry>,
    ): MunicipalityLocationKey? {
        if (BrazilianState.fromAbbreviation(stateAbbreviation) == null) {
            return null
        }
        return index[indexKey(stateAbbreviation, municipality)]?.locationKey
    }

    private fun indexKey(stateAbbreviation: String, municipality: String): CatalogIndexKey =
        CatalogIndexKey(
            stateAbbreviation = stateAbbreviation,
            normalizedName = MunicipalitySearchTextNormalizer.normalize(municipality),
        )

    private data class CatalogIndexKey(
        val stateAbbreviation: String,
        val normalizedName: String,
    )
}
