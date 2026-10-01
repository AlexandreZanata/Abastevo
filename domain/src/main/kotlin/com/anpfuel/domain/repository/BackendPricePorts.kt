package com.anpfuel.domain.repository

import com.anpfuel.domain.model.BackendPriceGroups
import com.anpfuel.domain.valueobject.FuelProduct

/**
 * P10-T02 backend read ports.
 *
 * HTTP is network-only (throws on transport/parse failure, never returns
 * cache); Room is cache-only. The application use case owns the
 * backend-down fallback so ANP offline paths stay untouched.
 */
interface BackendPriceHttpGateway {
    suspend fun fetchGroups(
        stationId: String,
        fuelProduct: FuelProduct?,
    ): BackendPriceGroups
}

interface BackendPriceCacheRepository {
    suspend fun load(
        stationId: String,
        fuelProduct: FuelProduct?,
    ): BackendPriceGroups?

    suspend fun save(groups: BackendPriceGroups)

    suspend fun clear()
}
