package com.anpfuel.domain.discovery

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.rule.MinimumSearchLengthRule
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct

/**
 * P20-T01 — Bounded community-first discovery query.
 *
 * Manual city + fuel browsing that works without GPS, account or photo
 * (B-BR-C06, BUC-C01). Carries no precise location, account id, photo or
 * private fields by construction. Read model reuses [BackendPriceGroup]
 * projection semantics (exact money, condition, dated ANP reference);
 * pagination is explicit offset-based with a bounded page size so future
 * indexed queries stay measurable before any new backend index.
 */
class DiscoveryQuery private constructor(
    val state: BrazilianState,
    val municipality: String,
    val fuelProduct: FuelProduct,
    val search: String?,
    val sort: DiscoverySort,
    val page: Int,
    val pageSize: Int,
) {
    fun offset(): Int = page * pageSize

    companion object {
        const val DEFAULT_PAGE_SIZE = 20
        const val MAX_PAGE_SIZE = 100

        fun create(
            state: BrazilianState?,
            municipality: String?,
            fuelProduct: FuelProduct?,
            search: String? = null,
            sort: DiscoverySort? = null,
            page: Int = 0,
            pageSize: Int = DEFAULT_PAGE_SIZE,
        ): DiscoveryQuery {
            if (state == null) throw DomainException("discovery state is required")
            if (fuelProduct == null) throw DomainException("discovery fuel is required")
            val city = municipality?.trim().orEmpty()
            if (city.isEmpty()) throw DomainException("discovery municipality is required")
            MinimumSearchLengthRule.validate(city)
            if (page < 0) throw DomainException("discovery page must be >= 0")
            if (pageSize !in 1..MAX_PAGE_SIZE) {
                throw DomainException("discovery pageSize must be 1..$MAX_PAGE_SIZE")
            }
            val normalizedSearch = search?.trim()?.takeIf { it.isNotEmpty() }
            if (normalizedSearch != null) {
                MinimumSearchLengthRule.validate(normalizedSearch)
            }
            return DiscoveryQuery(
                state = state,
                municipality = city,
                fuelProduct = fuelProduct,
                search = normalizedSearch,
                sort = sort ?: DiscoverySort.PRICE_ASC,
                page = page,
                pageSize = pageSize,
            )
        }
    }
}
