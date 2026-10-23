package com.anpfuel.domain.repository

/**
 * MEX-31 bounded public price-ranking reads (MEX-29 contract).
 *
 * Typed, anonymous, server-filtered pages. Implementations never
 * re-sort (stable ties preserved), never cache, and fail closed on
 * corrupt bodies. Municipal scope needs an exact IBGE code resolved
 * separately; unknown spellings refuse before any wire call.
 */
data class RankingQuery(
    val fuel: String,
    val unit: String,
    val condition: String = "STANDARD",
    val geography: String,
    val uf: String? = null,
    val ibgeCode: String? = null,
    val metric: String,
    val groupBy: String,
    val sort: String,
    val revision: String? = null,
    val limit: Int = 20,
    val cursor: String? = null,
)

data class RankingStation(
    val stationId: String,
    val amountMilliBrl: Long,
    val collectedOn: String,
    val state: String?,
    val ibgeCode: String?,
)

data class RankingArea(
    val level: String,
    val state: String,
    val ibgeCode: String,
    val meanNumerator: String,
    val meanDenominator: String,
    val minMilliBrl: Long,
    val maxMilliBrl: Long,
    val variationMilliBrl: Long,
    val observations: Long,
)

data class RankingPage(
    val stations: List<RankingStation>,
    val areas: List<RankingArea>,
    val nextCursor: String?,
    val revisionId: String,
    val generatedAt: String,
)

interface PriceRankingGateway {
    suspend fun rank(query: RankingQuery, cursor: String?): RankingPage
}

/**
 * Exact IBGE lookup for catalog-stored municipalities. Blank or
 * unmatched names return null; ambiguous names (same spelling in
 * several states) return null too — nothing is guessed.
 */
interface MunicipalityIbgeLookup {
    suspend fun lookup(stateAbbrev: String, normalizedName: String): String?

    suspend fun lookupAnywhere(normalizedName: String): String?
}
