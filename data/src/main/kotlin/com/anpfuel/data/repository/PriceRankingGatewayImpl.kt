package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.MunicipalityCatalogDao
import com.anpfuel.data.remote.ranking.PriceRankingHttpClient
import com.anpfuel.data.remote.ranking.PriceRankingParams
import com.anpfuel.domain.repository.MunicipalityIbgeLookup
import com.anpfuel.domain.repository.PriceRankingGateway
import com.anpfuel.domain.repository.RankingPage
import com.anpfuel.domain.repository.RankingQuery
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class PriceRankingGatewayImpl @Inject constructor(
    private val client: PriceRankingHttpClient,
) : PriceRankingGateway {

    override suspend fun rank(query: RankingQuery, cursor: String?): RankingPage {
        val page = client.rank(
            PriceRankingParams(
                fuel = query.fuel, unit = query.unit, condition = query.condition,
                geography = query.geography, uf = query.uf, ibgeCode = query.ibgeCode,
                metric = query.metric, groupBy = query.groupBy, sort = query.sort,
                revision = query.revision, limit = query.limit.coerceIn(1, 100), cursor = cursor,
            ),
        )
        return RankingPage(
            stations = page.stations.map {
                com.anpfuel.domain.repository.RankingStation(
                    it.stationId, it.amountMilliBrl, it.collectedOn, it.state, it.ibgeCode,
                )
            },
            areas = page.areas.map {
                com.anpfuel.domain.repository.RankingArea(
                    it.level, it.state, it.ibgeCode, it.meanNumerator, it.meanDenominator,
                    it.minMilliBrl, it.maxMilliBrl, it.variationMilliBrl, it.observations,
                )
            },
            nextCursor = page.nextCursor,
            revisionId = page.revisionId,
            generatedAt = page.generatedAt,
        )
    }
}

@Singleton
class MunicipalityIbgeLookupImpl @Inject constructor(
    private val catalog: MunicipalityCatalogDao,
) : MunicipalityIbgeLookup {

    override suspend fun lookup(stateAbbrev: String, normalizedName: String): String? {
        if (!Regex("^[A-Z]{2}$").matches(stateAbbrev)) return null
        val key = normalizedName.trim().uppercase().ifEmpty { return null }
        return catalog.findByStateAndNormalizedName(stateAbbrev, key)?.ibgeCode
    }

    override suspend fun lookupAnywhere(normalizedName: String): String? {
        val key = normalizedName.trim().uppercase().ifEmpty { return null }
        val matches = catalog.findByNormalizedNameAnywhere(key)
        if (matches.size != 1) return null
        return matches.single().ibgeCode
    }
}
