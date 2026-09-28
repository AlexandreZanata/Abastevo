package com.anpfuel.data.repository

import com.anpfuel.data.local.catalog.MunicipalityCatalogSeeder
import com.anpfuel.data.local.dao.MunicipalityCatalogDao
import com.anpfuel.data.local.dao.MunicipalityFtsDao
import com.anpfuel.data.local.fts.MunicipalityFtsMatchExpression
import com.anpfuel.data.local.model.MunicipalityFtsRow
import com.anpfuel.data.mapper.MunicipalityCatalogMapper
import com.anpfuel.domain.model.MunicipalitySearchResult
import com.anpfuel.domain.repository.MunicipalitySearchRepository
import com.anpfuel.domain.rule.IntelligentSearchRankingRule
import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.MunicipalityCatalogEntry
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class MunicipalitySearchRepositoryImpl @Inject constructor(
    private val municipalityFtsDao: MunicipalityFtsDao,
    private val municipalityCatalogDao: MunicipalityCatalogDao,
    private val catalogSeeder: MunicipalityCatalogSeeder,
) : MunicipalitySearchRepository {

    override suspend fun search(query: String, limit: Int): List<MunicipalitySearchResult> {
        catalogSeeder.seedIfEmpty()

        val matchQuery = MunicipalityFtsMatchExpression.fromUserQuery(query)
        if (matchQuery.isBlank()) {
            return emptyList()
        }

        val ftsRows = municipalityFtsDao.search(matchQuery, limit = FTS_PREFILTER_LIMIT)
        val candidates = searchFtsCandidates(query, ftsRows, limit)
        val ranked = IntelligentSearchRankingRule.rank(query, candidates)

        return ranked.take(limit).map { candidate ->
            MunicipalitySearchResult(
                municipality = candidate.entry.municipality,
                state = candidate.entry.state,
                dataAvailability = DataAvailability.HAS_DATA,
            )
        }
    }

    private suspend fun searchFtsCandidates(
        query: String,
        ftsRows: List<MunicipalityFtsRow>,
        limit: Int,
    ): List<MunicipalityCatalogEntry> {
        if (ftsRows.isEmpty()) {
            return fuzzyCatalogMatches(query)
        }

        val ftsMatches = resolveCatalogEntries(ftsRows)
        if (ftsMatches.isEmpty()) {
            return fuzzyCatalogMatches(query)
        }

        val rankedFtsMatches = IntelligentSearchRankingRule.rank(query, ftsMatches)
        if (rankedFtsMatches.size >= limit) {
            return ftsMatches
        }

        val ftsKeys = ftsRows.map { row -> row.state to row.municipality }.toSet()
        val supplementalMatches = fuzzyCatalogMatches(query).filter { entry ->
            entry.state.abbreviation to entry.municipality !in ftsKeys
        }

        return ftsMatches + supplementalMatches
    }

    private suspend fun resolveCatalogEntries(
        ftsRows: List<MunicipalityFtsRow>,
    ): List<MunicipalityCatalogEntry> {
        if (ftsRows.isEmpty()) {
            return emptyList()
        }

        val compositeKeys = ftsRows.map { row -> stateMunicipalityKey(row.state, row.municipality) }
        return municipalityCatalogDao.findByStateMunicipalityKeys(compositeKeys)
            .mapNotNull(MunicipalityCatalogMapper::toDomain)
    }

    private suspend fun fuzzyCatalogMatches(query: String): List<MunicipalityCatalogEntry> =
        municipalityCatalogDao.findAll()
            .mapNotNull(MunicipalityCatalogMapper::toDomain)
            .filter { entry ->
                IntelligentSearchRankingRule.classifyMatch(query, entry.municipality) != null
            }

    companion object {
        private const val FTS_PREFILTER_LIMIT = 200

        internal fun stateMunicipalityKey(state: String, municipality: String): String =
            "$state\u001f$municipality"
    }
}
