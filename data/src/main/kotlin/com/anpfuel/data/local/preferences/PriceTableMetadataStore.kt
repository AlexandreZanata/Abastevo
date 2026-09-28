package com.anpfuel.data.local.preferences

import com.anpfuel.domain.model.PriceTable

interface PriceTableMetadataStore {
    suspend fun findByUrl(sourceUrl: String): PriceTable?

    suspend fun save(priceTable: PriceTable)

    suspend fun clearAll()
}
