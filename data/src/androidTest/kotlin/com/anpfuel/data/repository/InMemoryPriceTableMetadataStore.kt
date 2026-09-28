package com.anpfuel.data.repository

import com.anpfuel.data.local.preferences.PriceTableMetadataStore
import com.anpfuel.domain.model.PriceTable

internal class InMemoryPriceTableMetadataStore : PriceTableMetadataStore {
    private val entries = mutableMapOf<String, PriceTable>()

    override suspend fun findByUrl(sourceUrl: String): PriceTable? = entries[sourceUrl]

    override suspend fun save(priceTable: PriceTable) {
        entries[priceTable.sourceUrl] = priceTable
    }

    override suspend fun clearAll() {
        entries.clear()
    }
}
