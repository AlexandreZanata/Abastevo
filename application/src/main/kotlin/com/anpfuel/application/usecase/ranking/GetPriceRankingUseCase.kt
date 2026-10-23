package com.anpfuel.application.usecase.ranking

import com.anpfuel.domain.exploration.PriceQuery
import com.anpfuel.domain.premium.PremiumValidationException
import com.anpfuel.domain.repository.MunicipalityIbgeLookup
import com.anpfuel.domain.repository.PriceRankingGateway
import com.anpfuel.domain.repository.RankingPage
import com.anpfuel.domain.repository.RankingQuery

/**
 * MEX-31 master ranking execution: a typed interpreted query becomes one
 * bounded server-filtered page. Municipal scope resolves the exact IBGE
 * code through the local catalog; unknown spellings refuse before any
 * wire call. Server order is preserved (stable ties are never re-sorted
 * client-side); stale cursors fail on the wire and keep held rows.
 */
class GetPriceRankingUseCase(
    private val gateway: PriceRankingGateway,
    private val catalog: MunicipalityIbgeLookup,
) {

    suspend operator fun invoke(query: PriceQuery, cursor: String?): RankingPage {
        val ibge = if (query.geography.level == "MUNICIPALITY") {
            val scoped = query.geography.uf.takeIf { it.isNotEmpty() }?.let { uf ->
                catalog.lookup(uf, query.geography.municipality)
            }
            // The interpreter normalizes to lowercase; the catalog keys are
            // uppercase. Ambiguous or absent spellings stay unresolved.
            val anywhere = if (scoped == null && query.geography.uf.isEmpty()) {
                catalog.lookupAnywhere(query.geography.municipality.uppercase())
            } else {
                scoped
            }
            anywhere ?: throw PremiumValidationException("unknown-municipality")
        } else {
            null
        }
        return gateway.rank(
            RankingQuery(
                fuel = query.product.fuel, unit = query.product.unit,
                condition = query.condition.kind.ifEmpty { "STANDARD" },
                geography = query.geography.level,
                uf = query.geography.uf.ifEmpty { null },
                ibgeCode = ibge,
                metric = query.metric.kind, groupBy = query.metric.groupBy, sort = query.metric.sort,
                revision = null, limit = query.page.limit.coerceIn(1, 100), cursor = cursor,
            ),
            cursor,
        )
    }
}
