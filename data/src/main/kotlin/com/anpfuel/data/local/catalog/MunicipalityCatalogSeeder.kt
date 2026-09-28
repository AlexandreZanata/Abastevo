package com.anpfuel.data.local.catalog

import android.content.Context
import androidx.room.withTransaction
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.dao.MunicipalityCatalogDao
import com.anpfuel.data.local.fts.MunicipalityFtsIndexer
import com.anpfuel.domain.rule.MunicipalityCatalogCompletenessRule
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class MunicipalityCatalogSeeder @Inject constructor(
    @ApplicationContext private val context: Context,
    private val database: AnpFuelDatabase,
    private val municipalityCatalogDao: MunicipalityCatalogDao,
    private val ftsIndexer: MunicipalityFtsIndexer,
) {

    suspend fun seedIfEmpty() {
        if (municipalityCatalogDao.count() > 0) {
            return
        }

        val json = context.assets.open(IbgeMunicipalityAsset.assetFileName())
            .bufferedReader()
            .use { it.readText() }
        val entities = IbgeMunicipalityAsset.parse(json).map { it.toEntity() }

        MunicipalityCatalogCompletenessRule.validate(entities.size)

        database.withTransaction {
            entities.chunked(BATCH_SIZE).forEach { batch ->
                municipalityCatalogDao.insertAll(batch)
            }
        }
        ftsIndexer.syncAfterCatalogChange()
    }

    companion object {
        const val BATCH_SIZE = 500
    }
}
