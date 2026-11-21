package com.anpfuel.data.di

import android.content.Context
import androidx.room.Room
import androidx.room.RoomDatabase
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.AnpFuelDatabaseMigrations
import com.anpfuel.data.local.dao.AveragePriceDao
import com.anpfuel.data.local.dao.BackendPriceCacheDao
import com.anpfuel.data.local.dao.ContributionOutboxDao
import com.anpfuel.data.local.dao.PhotoUploadSessionDao
import com.anpfuel.data.local.dao.ImportAuditLogDao
import com.anpfuel.data.local.dao.MunicipalityCatalogDao
import com.anpfuel.data.local.dao.MunicipalityFtsDao
import com.anpfuel.data.local.dao.ServerStationCacheDao
import com.anpfuel.data.local.dao.StationPriceDao
import com.anpfuel.data.local.dao.SurveyWeekDao
import com.anpfuel.data.local.dao.PremiumCursorDao
import com.anpfuel.data.local.dao.PremiumOutboxDao
import com.anpfuel.data.local.dao.VehicleDao
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

@Module
@InstallIn(SingletonComponent::class)
object DatabaseModule {

    private const val DATABASE_NAME = "anp_fuel.db"

    @Provides
    @Singleton
    fun provideAnpFuelDatabase(
        @ApplicationContext context: Context,
    ): AnpFuelDatabase =
        Room.databaseBuilder(context, AnpFuelDatabase::class.java, DATABASE_NAME)
            .setJournalMode(RoomDatabase.JournalMode.WRITE_AHEAD_LOGGING)
            .addMigrations(*AnpFuelDatabaseMigrations.all())
            .build()

    @Provides
    fun provideForumCacheDao(database: AnpFuelDatabase): com.anpfuel.data.local.dao.ForumCacheDao =
        database.forumCacheDao()

    @Provides
    fun provideDriverLocalDao(database: AnpFuelDatabase): com.anpfuel.data.local.dao.DriverLocalDao = database.driverLocalDao()

    @Provides
    fun provideSurveyWeekDao(database: AnpFuelDatabase): SurveyWeekDao =
        database.surveyWeekDao()

    @Provides
    fun provideMapPackageDao(database: AnpFuelDatabase): com.anpfuel.data.local.dao.MapPackageDao =
        database.mapPackageDao()

    @Provides
    fun provideAveragePriceDao(database: AnpFuelDatabase): AveragePriceDao =
        database.averagePriceDao()

    @Provides
    fun provideStationPriceDao(database: AnpFuelDatabase): StationPriceDao =
        database.stationPriceDao()

    @Provides
    fun provideImportAuditLogDao(database: AnpFuelDatabase): ImportAuditLogDao =
        database.importAuditLogDao()

    @Provides
    fun provideMunicipalityCatalogDao(database: AnpFuelDatabase): MunicipalityCatalogDao =
        database.municipalityCatalogDao()

    @Provides
    fun provideMunicipalityFtsDao(database: AnpFuelDatabase): MunicipalityFtsDao =
        database.municipalityFtsDao()

    @Provides
    fun provideVehicleDao(database: AnpFuelDatabase): VehicleDao =
        database.vehicleDao()

    @Provides
    fun provideVehicleCapabilityLeaseDao(database: AnpFuelDatabase): com.anpfuel.data.local.dao.VehicleCapabilityLeaseDao =
        database.vehicleCapabilityLeaseDao()

    @Provides
    @Singleton
    fun provideLedgerDraftDao(database: AnpFuelDatabase): com.anpfuel.data.local.dao.LedgerDraftDao =
        database.ledgerDraftDao()

    @Provides
    @Singleton
    fun provideLedgerErasedDao(database: AnpFuelDatabase): com.anpfuel.data.local.dao.LedgerErasedDao =
        database.ledgerErasedDao()

    @Provides
    fun provideChargingSnapshotDao(database: AnpFuelDatabase): com.anpfuel.data.local.dao.ChargingSnapshotDao =
        database.chargingSnapshotDao()

    @Provides
    fun provideBackendPriceCacheDao(database: AnpFuelDatabase): BackendPriceCacheDao =
        database.backendPriceCacheDao()

    @Provides
    fun provideContributionOutboxDao(database: AnpFuelDatabase): ContributionOutboxDao =
        database.contributionOutboxDao()

    @Provides
    fun providePhotoUploadSessionDao(database: AnpFuelDatabase): PhotoUploadSessionDao = database.photoUploadSessionDao()

    @Provides
    fun provideServerStationCacheDao(database: AnpFuelDatabase): ServerStationCacheDao =
        database.serverStationCacheDao()

    @Provides
    fun providePremiumOutboxDao(database: AnpFuelDatabase): PremiumOutboxDao =
        database.premiumOutboxDao()

    @Provides
    fun providePremiumCursorDao(database: AnpFuelDatabase): PremiumCursorDao =
        database.premiumCursorDao()
    @Provides
    fun qualityReportOutboxDao(database:AnpFuelDatabase):com.anpfuel.data.local.dao.QualityReportOutboxDao = database.qualityReportOutboxDao()

    @Provides
    fun provideCommunityFeedCacheDao(database: AnpFuelDatabase): com.anpfuel.data.local.dao.CommunityFeedCacheDao =
        database.communityFeedCacheDao()

}
