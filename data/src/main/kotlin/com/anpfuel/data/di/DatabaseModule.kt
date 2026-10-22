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
            .addMigrations(
                AnpFuelDatabaseMigrations.MIGRATION_1_2,
                AnpFuelDatabaseMigrations.MIGRATION_2_3,
                AnpFuelDatabaseMigrations.MIGRATION_3_4,
                AnpFuelDatabaseMigrations.MIGRATION_4_5,
                AnpFuelDatabaseMigrations.MIGRATION_5_6,
                AnpFuelDatabaseMigrations.MIGRATION_6_7,
                AnpFuelDatabaseMigrations.MIGRATION_7_8,
                AnpFuelDatabaseMigrations.MIGRATION_8_9,
                AnpFuelDatabaseMigrations.MIGRATION_9_10,
                AnpFuelDatabaseMigrations.MIGRATION_10_11,
                AnpFuelDatabaseMigrations.MIGRATION_11_12,
                AnpFuelDatabaseMigrations.MIGRATION_12_13,
                AnpFuelDatabaseMigrations.MIGRATION_13_14,
                AnpFuelDatabaseMigrations.MIGRATION_14_15,
                AnpFuelDatabaseMigrations.MIGRATION_15_16,
                AnpFuelDatabaseMigrations.MIGRATION_16_17,
                AnpFuelDatabaseMigrations.MIGRATION_17_18,
                AnpFuelDatabaseMigrations.MIGRATION_18_19,
                AnpFuelDatabaseMigrations.MIGRATION_19_20,
                AnpFuelDatabaseMigrations.MIGRATION_20_21,
                AnpFuelDatabaseMigrations.MIGRATION_21_22,
                AnpFuelDatabaseMigrations.MIGRATION_22_23,
            )
            .build()

    @Provides
    fun provideSurveyWeekDao(database: AnpFuelDatabase): SurveyWeekDao =
        database.surveyWeekDao()

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
    fun provideLedgerDraftDao(database: AnpFuelDatabase): com.anpfuel.data.local.dao.LedgerDraftDao =
        database.ledgerDraftDao()

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
