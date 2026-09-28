package com.anpfuel.data.di

import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.dao.AveragePriceDao
import com.anpfuel.data.local.dao.ImportAuditLogDao
import com.anpfuel.data.local.dao.MunicipalityFtsDao
import com.anpfuel.data.local.dao.StationPriceDao
import com.anpfuel.data.local.dao.SurveyWeekDao
import com.anpfuel.data.local.catalog.MunicipalityAnpAliasMerger
import com.anpfuel.data.local.catalog.MunicipalityCatalogSeeder
import com.anpfuel.data.local.fts.MunicipalityFtsIndexer
import com.anpfuel.data.local.importing.ImportAuditLogger
import com.anpfuel.data.local.importing.PriceTableBatchImporter
import com.anpfuel.data.local.preferences.AddressGeocodeCacheDataStore
import com.anpfuel.data.local.preferences.AddressGeocodeCacheStore
import com.anpfuel.data.local.preferences.GeocodeCacheDataStore
import com.anpfuel.data.local.preferences.GeocodeCacheStore
import com.anpfuel.data.local.preferences.DataStorePriceTableMetadataStore
import com.anpfuel.data.local.preferences.PriceTableMetadataStore
import com.anpfuel.data.repository.AddressGeocodeRepositoryImpl
import com.anpfuel.data.repository.AveragePriceRepositoryImpl
import com.anpfuel.data.repository.CacheRepositoryImpl
import com.anpfuel.data.repository.MunicipalityCatalogRepositoryImpl
import com.anpfuel.data.repository.MunicipalitySearchRepositoryImpl
import com.anpfuel.data.repository.NetworkConnectivityRepositoryImpl
import com.anpfuel.data.repository.NoOpDomainEventPublisher
import com.anpfuel.data.repository.PriceTableRepositoryImpl
import com.anpfuel.data.repository.PriceTableSyncGatewayImpl
import com.anpfuel.data.repository.StationPriceRepositoryImpl
import com.anpfuel.data.repository.StorageStatsRepositoryImpl
import com.anpfuel.data.repository.SyncJobRepositoryImpl
import com.anpfuel.data.repository.UserPreferencesRepositoryImpl
import com.anpfuel.data.repository.ReverseGeocodeRepositoryImpl
import com.anpfuel.data.repository.VehicleRepositoryImpl
import com.anpfuel.data.notification.PriceDropNotificationRepositoryImpl
import com.anpfuel.domain.repository.AddressGeocodeRepository
import com.anpfuel.domain.repository.ReverseGeocodeRepository
import com.anpfuel.domain.repository.VehicleRepository
import com.anpfuel.domain.repository.PriceDropNotificationRepository
import com.anpfuel.domain.repository.AveragePriceRepository
import com.anpfuel.domain.repository.CacheRepository
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.repository.MunicipalitySearchRepository
import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.PriceTableSyncGateway
import com.anpfuel.domain.repository.StationPriceRepository
import com.anpfuel.domain.repository.StorageStatsRepository
import com.anpfuel.domain.repository.SyncJobRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import dagger.Binds
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

@Module
@InstallIn(SingletonComponent::class)
abstract class RepositoryModule {

    @Binds
    @Singleton
    abstract fun bindPriceTableRepository(
        impl: PriceTableRepositoryImpl,
    ): PriceTableRepository

    @Binds
    @Singleton
    abstract fun bindAveragePriceRepository(
        impl: AveragePriceRepositoryImpl,
    ): AveragePriceRepository

    @Binds
    @Singleton
    abstract fun bindStationPriceRepository(
        impl: StationPriceRepositoryImpl,
    ): StationPriceRepository

    @Binds
    @Singleton
    abstract fun bindMunicipalityCatalogRepository(
        impl: MunicipalityCatalogRepositoryImpl,
    ): MunicipalityCatalogRepository

    @Binds
    @Singleton
    abstract fun bindMunicipalitySearchRepository(
        impl: MunicipalitySearchRepositoryImpl,
    ): MunicipalitySearchRepository

    @Binds
    @Singleton
    abstract fun bindGeocodeCacheStore(
        impl: GeocodeCacheDataStore,
    ): GeocodeCacheStore

    @Binds
    @Singleton
    abstract fun bindReverseGeocodeRepository(
        impl: ReverseGeocodeRepositoryImpl,
    ): ReverseGeocodeRepository

    @Binds
    @Singleton
    abstract fun bindAddressGeocodeCacheStore(
        impl: AddressGeocodeCacheDataStore,
    ): AddressGeocodeCacheStore

    @Binds
    @Singleton
    abstract fun bindAddressGeocodeRepository(
        impl: AddressGeocodeRepositoryImpl,
    ): AddressGeocodeRepository

    @Binds
    @Singleton
    abstract fun bindVehicleRepository(
        impl: VehicleRepositoryImpl,
    ): VehicleRepository

    @Binds
    @Singleton
    abstract fun bindPriceDropNotificationRepository(
        impl: PriceDropNotificationRepositoryImpl,
    ): PriceDropNotificationRepository

    @Binds
    @Singleton
    abstract fun bindUserPreferencesRepository(
        impl: UserPreferencesRepositoryImpl,
    ): UserPreferencesRepository

    @Binds
    @Singleton
    abstract fun bindSyncJobRepository(
        impl: SyncJobRepositoryImpl,
    ): SyncJobRepository

    @Binds
    @Singleton
    abstract fun bindCacheRepository(
        impl: CacheRepositoryImpl,
    ): CacheRepository

    @Binds
    @Singleton
    abstract fun bindStorageStatsRepository(
        impl: StorageStatsRepositoryImpl,
    ): StorageStatsRepository

    @Binds
    @Singleton
    abstract fun bindPriceTableSyncGateway(
        impl: PriceTableSyncGatewayImpl,
    ): PriceTableSyncGateway

    @Binds
    @Singleton
    abstract fun bindNetworkConnectivityGateway(
        impl: NetworkConnectivityRepositoryImpl,
    ): NetworkConnectivityGateway

    @Binds
    @Singleton
    abstract fun bindDomainEventPublisher(
        impl: NoOpDomainEventPublisher,
    ): DomainEventPublisher

    @Binds
    @Singleton
    abstract fun bindPriceTableMetadataStore(
        impl: DataStorePriceTableMetadataStore,
    ): PriceTableMetadataStore

    companion object {
        @Provides
        @Singleton
        fun provideImportAuditLogger(
            importAuditLogDao: ImportAuditLogDao,
        ): ImportAuditLogger = ImportAuditLogger(importAuditLogDao)

        @Provides
        @Singleton
        fun provideMunicipalityFtsIndexer(
            municipalityFtsDao: MunicipalityFtsDao,
        ): MunicipalityFtsIndexer = MunicipalityFtsIndexer(municipalityFtsDao)

        @Provides
        @Singleton
        fun providePriceTableBatchImporter(
            database: AnpFuelDatabase,
            surveyWeekDao: SurveyWeekDao,
            averagePriceDao: AveragePriceDao,
            stationPriceDao: StationPriceDao,
            importAuditLogger: ImportAuditLogger,
            catalogSeeder: MunicipalityCatalogSeeder,
            aliasMerger: MunicipalityAnpAliasMerger,
        ): PriceTableBatchImporter = PriceTableBatchImporter(
            database = database,
            surveyWeekDao = surveyWeekDao,
            averagePriceDao = averagePriceDao,
            stationPriceDao = stationPriceDao,
            importAuditLogger = importAuditLogger,
            catalogSeeder = catalogSeeder,
            aliasMerger = aliasMerger,
        )
    }
}
