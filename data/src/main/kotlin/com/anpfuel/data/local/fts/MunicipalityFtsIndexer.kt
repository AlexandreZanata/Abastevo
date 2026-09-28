package com.anpfuel.data.local.fts

import com.anpfuel.data.local.dao.MunicipalityFtsDao

/**
 * Keeps the external-content FTS index aligned after catalog seed or ANP alias merges.
 */
class MunicipalityFtsIndexer(
    private val municipalityFtsDao: MunicipalityFtsDao,
) {

    suspend fun syncAfterCatalogChange() {
        municipalityFtsDao.rebuildIndex()
    }

    suspend fun syncAfterBatchInsert() {
        syncAfterCatalogChange()
    }
}
