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
        com.anpfuel.data.local.entity.CommunityFeedCacheEntity::class,
        com.anpfuel.data.local.entity.ChargingSnapshotPlaceEntity::class,
        com.anpfuel.data.local.entity.ChargingSnapshotMetaEntity::class,
        com.anpfuel.data.local.entity.VehicleCapabilityLeaseEntity::class,
        com.anpfuel.data.local.entity.LedgerDraftEntity::class,
        com.anpfuel.data.local.entity.LedgerErasedEntity::class,
        com.anpfuel.data.local.map.MapGenerationEntity::class,
        com.anpfuel.data.local.map.MapPackageEntity::class,
        com.anpfuel.data.local.saved.SavedPlaceEntity::class,
        com.anpfuel.data.local.saved.SavedTripEntity::class,
        com.anpfuel.data.local.saved.SavedTripPointEntity::class,
        com.anpfuel.data.local.saved.SavedTombstoneEntity::class,
        com.anpfuel.data.local.saved.SavedImportEntity::class,
        com.anpfuel.data.local.entity.DriverShiftEntity::class,
        com.anpfuel.data.local.entity.DriverShiftGapEntity::class,
        com.anpfuel.data.local.entity.DriverEarningEntity::class,
        com.anpfuel.data.local.entity.DriverEarningRevisionEntity::class,
        com.anpfuel.data.local.entity.DriverDistanceEntity::class,
        com.anpfuel.data.local.entity.DriverLocalCommandEntity::class,
        com.anpfuel.data.local.entity.DriverDraftEntity::class,
    ],
    version = 30,
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

    abstract fun driverLocalDao(): com.anpfuel.data.local.dao.DriverLocalDao

    abstract fun backendPriceCacheDao(): BackendPriceCacheDao

    abstract fun contributionOutboxDao(): ContributionOutboxDao

    abstract fun photoUploadSessionDao(): PhotoUploadSessionDao

    abstract fun serverStationCacheDao(): ServerStationCacheDao

    abstract fun premiumOutboxDao(): PremiumOutboxDao

    abstract fun qualityReportOutboxDao():com.anpfuel.data.local.dao.QualityReportOutboxDao

    abstract fun premiumCursorDao(): PremiumCursorDao

    abstract fun communityFeedCacheDao(): com.anpfuel.data.local.dao.CommunityFeedCacheDao

    abstract fun chargingSnapshotDao(): com.anpfuel.data.local.dao.ChargingSnapshotDao

    abstract fun vehicleCapabilityLeaseDao(): com.anpfuel.data.local.dao.VehicleCapabilityLeaseDao

    abstract fun ledgerDraftDao(): com.anpfuel.data.local.dao.LedgerDraftDao

    abstract fun ledgerErasedDao(): com.anpfuel.data.local.dao.LedgerErasedDao

    abstract fun mapPackageDao(): com.anpfuel.data.local.dao.MapPackageDao

    abstract fun savedDao(): com.anpfuel.data.local.saved.SavedDao
}
