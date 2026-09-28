package com.anpfuel.domain.rule

import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.MunicipalityCatalogEntry
import com.anpfuel.domain.valueobject.SearchMatchType

/**
 * BR-017 — Intelligent search ranking.
 *
 * GIVEN query ≥ 2 chars
 * WHEN FTS runs
 * THEN exact prefix > accent-normalized match > substring
 * AND always tie-break by state abbreviation.
 */
object IntelligentSearchRankingRule {

    data class RankedCandidate(
        val entry: MunicipalityCatalogEntry,
        val matchType: SearchMatchType,
    )

    fun classifyMatch(query: String, municipality: String): SearchMatchType? {
        val trimmedQuery = query.trim()
        if (trimmedQuery.length < MinimumSearchLengthRule.MIN_LENGTH) {
            return null
        }

        val upperMunicipality = municipality.trim().uppercase()
        val upperQuery = trimmedQuery.uppercase()
        val normalizedMunicipality = MunicipalitySearchTextNormalizer.normalize(municipality)
        val normalizedQuery = MunicipalitySearchTextNormalizer.normalize(trimmedQuery)

        return when {
            upperMunicipality.startsWith(upperQuery) -> SearchMatchType.EXACT_PREFIX
            normalizedMunicipality.startsWith(normalizedQuery) -> SearchMatchType.ACCENT_NORMALIZED
            isTypoTolerantMatch(normalizedQuery, normalizedMunicipality) ->
                SearchMatchType.TYPO_TOLERANT
            normalizedMunicipality.contains(normalizedQuery) -> SearchMatchType.SUBSTRING
            else -> null
        }
    }

    internal fun isTypoTolerantMatch(normalizedQuery: String, normalizedMunicipality: String): Boolean {
        val queryTokens = normalizedQuery.split(' ').filter { it.isNotEmpty() }
        val municipalityTokens = normalizedMunicipality.split(' ').filter { it.isNotEmpty() }
        if (queryTokens.isEmpty() || municipalityTokens.size < queryTokens.size) {
            return false
        }

        return queryTokens.indices.all { index ->
            levenshteinDistance(queryTokens[index], municipalityTokens[index]) <= 1
        }
    }

    private fun levenshteinDistance(left: String, right: String): Int {
        if (left == right) return 0
        if (left.isEmpty()) return right.length
        if (right.isEmpty()) return left.length

        val costs = IntArray(right.length + 1) { it }
        for (i in left.indices) {
            var previousDiagonal = costs[0]
            costs[0] = i + 1
            for (j in right.indices) {
                val temp = costs[j + 1]
                val substitutionCost = if (left[i] == right[j]) previousDiagonal else previousDiagonal + 1
                costs[j + 1] = minOf(
                    costs[j + 1] + 1,
                    costs[j] + 1,
                    substitutionCost,
                )
                previousDiagonal = temp
            }
        }
        return costs[right.length]
    }

    fun rank(
        query: String,
        candidates: List<MunicipalityCatalogEntry>,
    ): List<RankedCandidate> =
        candidates.mapNotNull { entry ->
            classifyMatch(query, entry.municipality)?.let { matchType ->
                RankedCandidate(entry = entry, matchType = matchType)
            }
        }.sortedWith(comparator(query))

    private fun comparator(query: String): Comparator<RankedCandidate> {
        val queryTokenCount = MunicipalitySearchTextNormalizer.normalize(query)
            .split(' ')
            .filter { it.isNotEmpty() }
            .size

        return compareBy<RankedCandidate> { it.matchType.rank }
            .thenBy { tokenCountDistance(it.entry.municipality, queryTokenCount) }
            .thenBy { MunicipalitySearchTextNormalizer.normalize(it.entry.municipality).split(' ').size }
            .thenBy { stateTieBreakRank(it.entry.state, query) }
            .thenBy { MunicipalitySearchTextNormalizer.normalize(it.entry.municipality) }
    }

    private fun tokenCountDistance(municipality: String, queryTokenCount: Int): Int {
        val municipalityTokenCount = MunicipalitySearchTextNormalizer.normalize(municipality)
            .split(' ')
            .filter { it.isNotEmpty() }
            .size
        return kotlin.math.abs(municipalityTokenCount - queryTokenCount)
    }

    private fun stateTieBreakRank(state: BrazilianState, query: String): String =
        state.abbreviation
}
