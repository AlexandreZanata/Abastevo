package com.anpfuel.data.repository

import androidx.room.withTransaction
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.dao.AveragePriceDao
import com.anpfuel.data.local.dao.ImportAuditLogDao
import com.anpfuel.data.local.dao.StationPriceDao
import com.anpfuel.data.local.dao.SurveyWeekDao
import com.anpfuel.data.local.fts.MunicipalityFtsIndexer
import com.anpfuel.data.local.preferences.PriceTableMetadataStore
import com.anpfuel.data.local.preferences.SyncStateDataStore
import com.anpfuel.domain.repository.CacheRepository
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class CacheRepositoryImpl @Inject constructor(
    private val database: AnpFuelDatabase,
    private val surveyWeekDao: SurveyWeekDao,
    private val averagePriceDao: AveragePriceDao,
    private val stationPriceDao: StationPriceDao,
    private val importAuditLogDao: ImportAuditLogDao,
    private val priceTableMetadataStore: PriceTableMetadataStore,
    private val syncStateDataStore: SyncStateDataStore,
    private val ftsIndexer: MunicipalityFtsIndexer,
) : CacheRepository {

    override suspend fun clearAllImportedData() {
        database.withTransaction {
            stationPriceDao.deleteAll()
            averagePriceDao.deleteAll()
            surveyWeekDao.deleteAll()
            importAuditLogDao.deleteAll()
        }
        ftsIndexer.syncAfterBatchInsert()
        priceTableMetadataStore.clearAll()
        syncStateDataStore.resetToIdle()
    }
}
