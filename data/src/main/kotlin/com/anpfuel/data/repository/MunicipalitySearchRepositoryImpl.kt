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
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withContext

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
        currentCoroutineContext().ensureActive()
        val candidates = searchCandidates(query, matchQuery, ftsRows, limit)
        // MEX-05: approximate ranking over a bounded pool stays off the main thread.
        val ranked = withContext(Dispatchers.Default) {
            currentCoroutineContext().ensureActive()
            IntelligentSearchRankingRule.rank(query, candidates)
        }

        return ranked.take(limit).map { candidate ->
            MunicipalitySearchResult(
                municipality = candidate.entry.municipality,
                state = candidate.entry.state,
                dataAvailability = DataAvailability.HAS_DATA,
            )
        }
    }

    private suspend fun searchCandidates(
        query: String,
        matchQuery: String,
        ftsRows: List<MunicipalityFtsRow>,
        limit: Int,
    ): List<MunicipalityCatalogEntry> {
        if (ftsRows.isEmpty()) {
            return supplementalMatches(matchQuery, emptySet())
        }

        val ftsMatches = resolveCatalogEntries(ftsRows)
        if (ftsMatches.isEmpty()) {
            return supplementalMatches(matchQuery, emptySet())
        }

        // Fast path unchanged: enough ranked FTS matches, no supplement read.
        val rankedFtsMatches = withContext(Dispatchers.Default) {
            IntelligentSearchRankingRule.rank(query, ftsMatches)
        }
        if (rankedFtsMatches.size >= limit) {
            return ftsMatches
        }

        val ftsKeys = ftsRows.map { row -> row.state to row.municipality }.toSet()
        return ftsMatches + supplementalMatches(matchQuery, ftsKeys)
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

    /**
     * MEX-05 bounded supplement: indexed normalized-prefix ranges per stem
     * instead of the retired per-keystroke full-catalog scan. Covers exact,
     * prefix, accent and single-edit typo candidates that the FTS tokenizer
     * cannot see; final ranking still filters by match type.
     */
    private suspend fun supplementalMatches(
        matchQuery: String,
        excludeKeys: Set<Pair<String, String>>,
    ): List<MunicipalityCatalogEntry> {
        val stems = supplementStems(matchQuery)
        if (stems.isEmpty()) {
            return emptyList()
        }
        val seen = mutableSetOf<Pair<String, String>>()
        val pooled = mutableListOf<MunicipalityCatalogEntry>()
        for (stem in stems) {
            currentCoroutineContext().ensureActive()
            for (entity in municipalityCatalogDao.findByNormalizedPrefix(
                rangeStart = stem.start,
                rangeEnd = stem.end,
                limit = SUPPLEMENT_LIMIT,
            )) {
                val entry = MunicipalityCatalogMapper.toDomain(entity) ?: continue
                val key = entry.state.abbreviation to entry.municipality
                if (key in excludeKeys || !seen.add(key)) {
                    continue
                }
                pooled.add(entry)
            }
        }
        return pooled
    }

    companion object {
        private const val FTS_PREFILTER_LIMIT = 200

        /**
         * National worst-case stem pool is 756 rows ("S*"), so 1,000 keeps
         * every stem fully covered with headroom; multi-stem unions stay
         * bounded and deterministic via ORDER BY + LIMIT in SQL.
         */
        internal const val SUPPLEMENT_LIMIT = 1000

        internal fun stateMunicipalityKey(state: String, municipality: String): String =
            "$state\u001f$municipality"

        /**
         * Stems for the supplement pool, derived from the already-normalized
         * FTS tokens. Short tokens use one character (broader recall for
         * single edits at index 1); longer tokens use two (tighter pools).
         * Single-character tokens are left to FTS prefix matching. At most
         * the first two tokens participate, keeping reads bounded.
         */
        internal fun supplementStems(matchQuery: String): List<PrefixRange> =
            matchQuery.split(' ')
                .map { it.removeSuffix("*") }
                .filter { it.length >= 2 }
                .take(2)
                .map { token ->
                    val stem = if (token.length >= 4) token.take(2) else token.take(1)
                    PrefixRange(stem, stem + MAX_UNICODE_CHAR)
                }

        internal data class PrefixRange(
            val start: String,
            val end: String,
        )

        private const val MAX_UNICODE_CHAR = "\uFFFF"
    }
}
