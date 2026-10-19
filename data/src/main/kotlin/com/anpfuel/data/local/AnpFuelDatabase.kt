package com.anpfuel.data.local

import com.anpfuel.data.local.dao.PhotoUploadSessionDao
import com.anpfuel.data.local.dao.PremiumCursorDao
import com.anpfuel.data.local.dao.PremiumOutboxDao
import com.anpfuel.data.local.entity.PremiumCursorEntity
import com.anpfuel.data.local.entity.PremiumOutboxEntity
import com.anpfuel.data.local.entity.PhotoUploadSessionEntity
import androidx.room.Database
import androidx.room.RoomDatabase
import com.anpfuel.data.local.dao.AveragePriceDao
import com.anpfuel.data.local.dao.BackendPriceCacheDao
import com.anpfuel.data.local.dao.ContributionOutboxDao
import com.anpfuel.data.local.dao.ImportAuditLogDao
import com.anpfuel.data.local.dao.MunicipalityCatalogDao
import com.anpfuel.data.local.dao.MunicipalityFtsDao
import com.anpfuel.data.local.dao.ServerStationCacheDao
import com.anpfuel.data.local.dao.StationPriceDao
import com.anpfuel.data.local.dao.SurveyWeekDao
import com.anpfuel.data.local.dao.VehicleDao
import com.anpfuel.data.local.entity.AveragePriceEntity
import com.anpfuel.data.local.entity.BackendPriceCacheEntity
import com.anpfuel.data.local.entity.ContributionOutboxEntity
import com.anpfuel.data.local.entity.ImportAuditLogEntity
import com.anpfuel.data.local.entity.MunicipalityCatalogEntity
import com.anpfuel.data.local.entity.MunicipalityFtsEntity
import com.anpfuel.data.local.entity.ServerCatalogMetaEntity
import com.anpfuel.data.local.entity.ServerStationCacheEntity
import com.anpfuel.data.local.entity.StationPriceEntity
import com.anpfuel.data.local.entity.SurveyWeekEntity
import com.anpfuel.data.local.entity.VehicleEntity

@Database(
    entities = [
        SurveyWeekEntity::class,
        AveragePriceEntity::class,
        StationPriceEntity::class,
        ImportAuditLogEntity::class,
        MunicipalityCatalogEntity::class,
        MunicipalityFtsEntity::class,
        VehicleEntity::class,
        BackendPriceCacheEntity::class,
        ContributionOutboxEntity::class,
        PhotoUploadSessionEntity::class,
        ServerStationCacheEntity::class,
        ServerCatalogMetaEntity::class,
        PremiumOutboxEntity::class,
        PremiumCursorEntity::class,
        com.anpfuel.data.local.entity.QualityOwnerGateEntity::class,
        com.anpfuel.data.local.entity.QualityReportOutboxEntity::class,
    ],
    version = 15,
    exportSchema = true,
)
abstract class AnpFuelDatabase : RoomDatabase() {

    abstract fun surveyWeekDao(): SurveyWeekDao

    abstract fun averagePriceDao(): AveragePriceDao

    abstract fun stationPriceDao(): StationPriceDao

    abstract fun importAuditLogDao(): ImportAuditLogDao

    abstract fun municipalityCatalogDao(): MunicipalityCatalogDao

    abstract fun municipalityFtsDao(): MunicipalityFtsDao

    abstract fun vehicleDao(): VehicleDao

    abstract fun backendPriceCacheDao(): BackendPriceCacheDao

    abstract fun contributionOutboxDao(): ContributionOutboxDao

    abstract fun photoUploadSessionDao(): PhotoUploadSessionDao

    abstract fun serverStationCacheDao(): ServerStationCacheDao

    abstract fun premiumOutboxDao(): PremiumOutboxDao

    abstract fun qualityReportOutboxDao():com.anpfuel.data.local.dao.QualityReportOutboxDao

    abstract fun premiumCursorDao(): PremiumCursorDao
}
