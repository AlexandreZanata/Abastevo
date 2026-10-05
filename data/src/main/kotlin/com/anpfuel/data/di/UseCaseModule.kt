package com.anpfuel.data.di

import com.anpfuel.application.port.AnonymousContributionFlagProvider
import com.anpfuel.application.port.AnonymousDeviceKeyPort
import com.anpfuel.application.port.CaptureOcrFlagProvider
import com.anpfuel.application.port.CommunityReadsFlagProvider
import com.anpfuel.application.port.CommunityVoteFlagProvider
import com.anpfuel.application.port.ContributionOutboxFlagProvider
import com.anpfuel.application.port.FeedbackFlagProvider
import com.anpfuel.application.port.OcrPort
import com.anpfuel.application.usecase.alert.ConfigurePriceDropAlertUseCase
import com.anpfuel.application.usecase.alert.EvaluatePriceDropAlertsUseCase
import com.anpfuel.application.usecase.capture.ConfirmPriceCaptureUseCase
import com.anpfuel.application.usecase.community.GetCommunityPriceGroupsUseCase
import com.anpfuel.application.usecase.community.SubmitCommunityVoteUseCase
import com.anpfuel.application.usecase.directory.GetServerStationDetailUseCase
import com.anpfuel.application.usecase.directory.GetNearbyServerStationsUseCase
import com.anpfuel.application.usecase.directory.GetServerStationsUseCase
import com.anpfuel.application.usecase.contribution.CancelOwnedContributionUseCase
import com.anpfuel.application.usecase.contribution.EnqueueContributionUseCase
import com.anpfuel.application.usecase.contribution.GetOwnedContributionsUseCase
import com.anpfuel.application.usecase.feedback.GetFeedbackPageUseCase
import com.anpfuel.application.usecase.feedback.SubmitFeedbackUseCase
import com.anpfuel.application.usecase.identity.AnonymousDeviceFlow
import com.anpfuel.domain.repository.BackendPriceCacheRepository
import com.anpfuel.domain.repository.BackendPriceHttpGateway
import com.anpfuel.domain.repository.ServerStationCache
import com.anpfuel.domain.repository.ServerStationGateway
import com.anpfuel.domain.repository.CommunityVoteGateway
import com.anpfuel.domain.repository.ContributionOutboxRepository
import com.anpfuel.domain.repository.FeedbackCacheRepository
import com.anpfuel.domain.repository.FeedbackGateway
import com.anpfuel.domain.repository.FeedbackOutboxPort
import com.anpfuel.application.usecase.location.SearchMunicipalityUseCase
import com.anpfuel.application.usecase.location.ResolveDeviceLocationUseCase
import com.anpfuel.application.usecase.location.SelectLocationUseCase
import com.anpfuel.application.usecase.navigation.ResolveAppStartDestinationUseCase
import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.application.usecase.onboarding.CompleteLocationPromptUseCase
import com.anpfuel.application.usecase.onboarding.CompleteOnboardingUseCase
import com.anpfuel.application.usecase.onboarding.OnboardingSelectWeekAndSyncUseCase
import com.anpfuel.application.usecase.readiness.GetDataReadinessUseCase
import com.anpfuel.application.usecase.price.GetMunicipalityPricesUseCase
import com.anpfuel.application.usecase.price.GetPriceHistoryUseCase
import com.anpfuel.application.usecase.price.GetStationPricesUseCase
import com.anpfuel.application.usecase.settings.ApplyStationDetailRetentionUseCase
import com.anpfuel.application.usecase.settings.ClearCacheUseCase
import com.anpfuel.application.usecase.settings.GetSettingsUseCase
import com.anpfuel.application.usecase.settings.GetStorageUsageUseCase
import com.anpfuel.application.usecase.settings.UpdatePreferencesUseCase
import com.anpfuel.application.usecase.station.BuildStationNavigationQueryUseCase
import com.anpfuel.application.usecase.station.FindNearestBestPriceStationUseCase
import com.anpfuel.application.usecase.sync.AutoDownloadLatestWeekUseCase
import com.anpfuel.application.usecase.sync.DiscoverSurveyWeekCatalogUseCase
import com.anpfuel.application.usecase.sync.DownloadStationDetailUseCase
import com.anpfuel.application.usecase.sync.SelectSurveyWeekUseCase
import com.anpfuel.application.usecase.sync.SelectWeekAndSyncUseCase
import com.anpfuel.application.sync.SyncExecutionLock
import com.anpfuel.application.usecase.sync.SyncPriceTablesUseCase
import com.anpfuel.application.usecase.vehicle.DeleteVehicleUseCase
import com.anpfuel.application.usecase.vehicle.GetVehicleUseCase
import com.anpfuel.application.usecase.vehicle.GetTankFillCostEstimatesUseCase
import com.anpfuel.application.usecase.vehicle.ListVehiclesUseCase
import com.anpfuel.application.usecase.vehicle.SaveVehicleUseCase
import com.anpfuel.domain.repository.AddressGeocodeRepository
import com.anpfuel.domain.repository.PriceDropAlertHistoryRepository
import com.anpfuel.domain.repository.PriceDropNotificationRepository
import com.anpfuel.domain.repository.ReverseGeocodeRepository
import com.anpfuel.domain.repository.VehicleRepository
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
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

@Module
@InstallIn(SingletonComponent::class)
object UseCaseModule {

    @Provides
    @Singleton
    fun provideResolveAppStartDestinationUseCase(
        userPreferencesRepository: UserPreferencesRepository,
        priceTableRepository: PriceTableRepository,
    ): ResolveAppStartDestinationUseCase = ResolveAppStartDestinationUseCase(
        userPreferencesRepository = userPreferencesRepository,
        priceTableRepository = priceTableRepository,
    )

    @Provides
    @Singleton
    fun provideApplyStationDetailRetentionUseCase(
        userPreferencesRepository: UserPreferencesRepository,
        stationPriceRepository: StationPriceRepository,
    ): ApplyStationDetailRetentionUseCase = ApplyStationDetailRetentionUseCase(
        userPreferencesRepository = userPreferencesRepository,
        stationPriceRepository = stationPriceRepository,
    )

    @Provides
    @Singleton
    fun provideDiscoverSurveyWeekCatalogUseCase(
        priceTableSyncGateway: PriceTableSyncGateway,
    ): DiscoverSurveyWeekCatalogUseCase = DiscoverSurveyWeekCatalogUseCase(
        priceTableSyncGateway = priceTableSyncGateway,
    )

    @Provides
    @Singleton
    fun provideSelectSurveyWeekUseCase(
        userPreferencesRepository: UserPreferencesRepository,
        eventPublisher: DomainEventPublisher,
    ): SelectSurveyWeekUseCase = SelectSurveyWeekUseCase(
        userPreferencesRepository = userPreferencesRepository,
        eventPublisher = eventPublisher,
    )

    @Provides
    @Singleton
    fun provideSyncExecutionLock(): SyncExecutionLock = SyncExecutionLock()

    @Provides
    @Singleton
    fun provideSyncPriceTablesUseCase(
        syncJobRepository: SyncJobRepository,
        priceTableRepository: PriceTableRepository,
        priceTableSyncGateway: PriceTableSyncGateway,
        userPreferencesRepository: UserPreferencesRepository,
        eventPublisher: DomainEventPublisher,
        applyStationDetailRetentionUseCase: ApplyStationDetailRetentionUseCase,
        syncExecutionLock: SyncExecutionLock,
    ): SyncPriceTablesUseCase = SyncPriceTablesUseCase(
        syncJobRepository = syncJobRepository,
        priceTableRepository = priceTableRepository,
        priceTableSyncGateway = priceTableSyncGateway,
        userPreferencesRepository = userPreferencesRepository,
        eventPublisher = eventPublisher,
        applyStationDetailRetentionUseCase = applyStationDetailRetentionUseCase,
        syncExecutionLock = syncExecutionLock,
    )

    @Provides
    @Singleton
    fun provideDownloadStationDetailUseCase(
        syncJobRepository: SyncJobRepository,
        priceTableRepository: PriceTableRepository,
        priceTableSyncGateway: PriceTableSyncGateway,
        userPreferencesRepository: UserPreferencesRepository,
        eventPublisher: DomainEventPublisher,
    ): DownloadStationDetailUseCase = DownloadStationDetailUseCase(
        syncJobRepository = syncJobRepository,
        priceTableRepository = priceTableRepository,
        priceTableSyncGateway = priceTableSyncGateway,
        userPreferencesRepository = userPreferencesRepository,
        eventPublisher = eventPublisher,
    )

    @Provides
    @Singleton
    fun provideGetDataReadinessUseCase(
        priceTableRepository: PriceTableRepository,
        syncJobRepository: SyncJobRepository,
    ): GetDataReadinessUseCase = GetDataReadinessUseCase(
        priceTableRepository = priceTableRepository,
        syncJobRepository = syncJobRepository,
    )

    @Provides
    @Singleton
    fun provideObserveNetworkConnectivityUseCase(
        networkConnectivityGateway: NetworkConnectivityGateway,
    ): ObserveNetworkConnectivityUseCase = ObserveNetworkConnectivityUseCase(
        networkConnectivityGateway = networkConnectivityGateway,
    )

    @Provides
    @Singleton
    fun provideCompleteLocationPromptUseCase(
        userPreferencesRepository: UserPreferencesRepository,
        eventPublisher: DomainEventPublisher,
    ): CompleteLocationPromptUseCase = CompleteLocationPromptUseCase(
        userPreferencesRepository = userPreferencesRepository,
        eventPublisher = eventPublisher,
    )

    @Provides
    @Singleton
    fun provideCompleteOnboardingUseCase(
        userPreferencesRepository: UserPreferencesRepository,
        priceTableRepository: PriceTableRepository,
    ): CompleteOnboardingUseCase = CompleteOnboardingUseCase(
        userPreferencesRepository = userPreferencesRepository,
        priceTableRepository = priceTableRepository,
    )

    @Provides
    @Singleton
    fun provideAutoDownloadLatestWeekUseCase(
        userPreferencesRepository: UserPreferencesRepository,
        priceTableRepository: PriceTableRepository,
        discoverSurveyWeekCatalogUseCase: DiscoverSurveyWeekCatalogUseCase,
        selectWeekAndSyncUseCase: SelectWeekAndSyncUseCase,
    ): AutoDownloadLatestWeekUseCase = AutoDownloadLatestWeekUseCase(
        userPreferencesRepository = userPreferencesRepository,
        priceTableRepository = priceTableRepository,
        discoverSurveyWeekCatalogUseCase = discoverSurveyWeekCatalogUseCase,
        selectWeekAndSyncUseCase = selectWeekAndSyncUseCase,
    )

    @Provides
    @Singleton
    fun provideSelectWeekAndSyncUseCase(
        selectSurveyWeekUseCase: SelectSurveyWeekUseCase,
        syncPriceTablesUseCase: SyncPriceTablesUseCase,
    ): SelectWeekAndSyncUseCase = SelectWeekAndSyncUseCase(
        selectSurveyWeekUseCase = selectSurveyWeekUseCase,
        syncPriceTablesUseCase = syncPriceTablesUseCase,
    )

    @Provides
    @Singleton
    fun provideOnboardingSelectWeekAndSyncUseCase(
        selectSurveyWeekUseCase: SelectSurveyWeekUseCase,
        syncPriceTablesUseCase: SyncPriceTablesUseCase,
        completeOnboardingUseCase: CompleteOnboardingUseCase,
    ): OnboardingSelectWeekAndSyncUseCase = OnboardingSelectWeekAndSyncUseCase(
        selectSurveyWeekUseCase = selectSurveyWeekUseCase,
        syncPriceTablesUseCase = syncPriceTablesUseCase,
        completeOnboardingUseCase = completeOnboardingUseCase,
    )

    @Provides
    @Singleton
    fun provideSearchMunicipalityUseCase(
        municipalitySearchRepository: MunicipalitySearchRepository,
        municipalityCatalogRepository: MunicipalityCatalogRepository,
        priceTableRepository: PriceTableRepository,
        userPreferencesRepository: UserPreferencesRepository,
    ): SearchMunicipalityUseCase = SearchMunicipalityUseCase(
        municipalitySearchRepository = municipalitySearchRepository,
        municipalityCatalogRepository = municipalityCatalogRepository,
        priceTableRepository = priceTableRepository,
        userPreferencesRepository = userPreferencesRepository,
    )

    @Provides
    @Singleton
    fun provideSelectLocationUseCase(
        municipalityCatalogRepository: MunicipalityCatalogRepository,
        priceTableRepository: PriceTableRepository,
        userPreferencesRepository: UserPreferencesRepository,
        eventPublisher: DomainEventPublisher,
    ): SelectLocationUseCase = SelectLocationUseCase(
        municipalityCatalogRepository = municipalityCatalogRepository,
        priceTableRepository = priceTableRepository,
        userPreferencesRepository = userPreferencesRepository,
        eventPublisher = eventPublisher,
    )

    @Provides
    @Singleton
    fun provideResolveDeviceLocationUseCase(
        reverseGeocodeRepository: ReverseGeocodeRepository,
        selectLocationUseCase: SelectLocationUseCase,
        eventPublisher: DomainEventPublisher,
    ): ResolveDeviceLocationUseCase = ResolveDeviceLocationUseCase(
        reverseGeocodeRepository = reverseGeocodeRepository,
        selectLocationUseCase = selectLocationUseCase,
        eventPublisher = eventPublisher,
    )

    @Provides
    @Singleton
    fun provideGetMunicipalityPricesUseCase(
        averagePriceRepository: AveragePriceRepository,
        municipalityCatalogRepository: MunicipalityCatalogRepository,
        priceTableRepository: PriceTableRepository,
        userPreferencesRepository: UserPreferencesRepository,
    ): GetMunicipalityPricesUseCase = GetMunicipalityPricesUseCase(
        averagePriceRepository = averagePriceRepository,
        municipalityCatalogRepository = municipalityCatalogRepository,
        priceTableRepository = priceTableRepository,
        userPreferencesRepository = userPreferencesRepository,
    )

    @Provides
    @Singleton
    fun provideGetPriceHistoryUseCase(
        averagePriceRepository: AveragePriceRepository,
        priceTableRepository: PriceTableRepository,
        userPreferencesRepository: UserPreferencesRepository,
    ): GetPriceHistoryUseCase = GetPriceHistoryUseCase(
        averagePriceRepository = averagePriceRepository,
        priceTableRepository = priceTableRepository,
        userPreferencesRepository = userPreferencesRepository,
    )

    @Provides
    @Singleton
    fun provideBuildStationNavigationQueryUseCase(
        userPreferencesRepository: UserPreferencesRepository,
        eventPublisher: DomainEventPublisher,
    ): BuildStationNavigationQueryUseCase = BuildStationNavigationQueryUseCase(
        userPreferencesRepository = userPreferencesRepository,
        eventPublisher = eventPublisher,
    )

    @Provides
    @Singleton
    fun provideFindNearestBestPriceStationUseCase(
        getStationPricesUseCase: GetStationPricesUseCase,
        addressGeocodeRepository: AddressGeocodeRepository,
        buildStationNavigationQueryUseCase: BuildStationNavigationQueryUseCase,
        userPreferencesRepository: UserPreferencesRepository,
    ): FindNearestBestPriceStationUseCase = FindNearestBestPriceStationUseCase(
        getStationPricesUseCase = getStationPricesUseCase,
        addressGeocodeRepository = addressGeocodeRepository,
        buildStationNavigationQueryUseCase = buildStationNavigationQueryUseCase,
        userPreferencesRepository = userPreferencesRepository,
    )

    @Provides
    @Singleton
    fun provideGetStationPricesUseCase(
        stationPriceRepository: StationPriceRepository,
        priceTableRepository: PriceTableRepository,
        userPreferencesRepository: UserPreferencesRepository,
    ): GetStationPricesUseCase = GetStationPricesUseCase(
        stationPriceRepository = stationPriceRepository,
        priceTableRepository = priceTableRepository,
        userPreferencesRepository = userPreferencesRepository,
    )

    @Provides
    @Singleton
    fun provideGetSettingsUseCase(
        userPreferencesRepository: UserPreferencesRepository,
    ): GetSettingsUseCase = GetSettingsUseCase(
        userPreferencesRepository = userPreferencesRepository,
    )

    @Provides
    @Singleton
    fun provideGetStorageUsageUseCase(
        storageStatsRepository: StorageStatsRepository,
    ): GetStorageUsageUseCase = GetStorageUsageUseCase(
        storageStatsRepository = storageStatsRepository,
    )

    @Provides
    @Singleton
    fun provideUpdatePreferencesUseCase(
        userPreferencesRepository: UserPreferencesRepository,
        eventPublisher: DomainEventPublisher,
    ): UpdatePreferencesUseCase = UpdatePreferencesUseCase(
        userPreferencesRepository = userPreferencesRepository,
        eventPublisher = eventPublisher,
    )

    @Provides
    @Singleton
    fun provideClearCacheUseCase(
        cacheRepository: CacheRepository,
        stationPriceRepository: StationPriceRepository,
        userPreferencesRepository: UserPreferencesRepository,
        eventPublisher: DomainEventPublisher,
    ): ClearCacheUseCase = ClearCacheUseCase(
        cacheRepository = cacheRepository,
        stationPriceRepository = stationPriceRepository,
        userPreferencesRepository = userPreferencesRepository,
        eventPublisher = eventPublisher,
    )

    @Provides
    @Singleton
    fun provideGetTankFillCostEstimatesUseCase(
        averagePriceRepository: AveragePriceRepository,
        stationPriceRepository: StationPriceRepository,
        priceTableRepository: PriceTableRepository,
        userPreferencesRepository: UserPreferencesRepository,
    ): GetTankFillCostEstimatesUseCase = GetTankFillCostEstimatesUseCase(
        averagePriceRepository = averagePriceRepository,
        stationPriceRepository = stationPriceRepository,
        priceTableRepository = priceTableRepository,
        userPreferencesRepository = userPreferencesRepository,
    )

    @Provides
    @Singleton
    fun provideListVehiclesUseCase(
        vehicleRepository: VehicleRepository,
    ): ListVehiclesUseCase = ListVehiclesUseCase(
        vehicleRepository = vehicleRepository,
    )

    @Provides
    @Singleton
    fun provideGetVehicleUseCase(
        vehicleRepository: VehicleRepository,
    ): GetVehicleUseCase = GetVehicleUseCase(
        vehicleRepository = vehicleRepository,
    )

    @Provides
    @Singleton
    fun provideSaveVehicleUseCase(
        vehicleRepository: VehicleRepository,
        eventPublisher: DomainEventPublisher,
    ): SaveVehicleUseCase = SaveVehicleUseCase(
        vehicleRepository = vehicleRepository,
        eventPublisher = eventPublisher,
    )

    @Provides
    @Singleton
    fun provideDeleteVehicleUseCase(
        vehicleRepository: VehicleRepository,
        eventPublisher: DomainEventPublisher,
    ): DeleteVehicleUseCase = DeleteVehicleUseCase(
        vehicleRepository = vehicleRepository,
        eventPublisher = eventPublisher,
    )

    @Provides
    @Singleton
    fun provideEvaluatePriceDropAlertsUseCase(
        vehicleRepository: VehicleRepository,
        averagePriceRepository: AveragePriceRepository,
        stationPriceRepository: StationPriceRepository,
        priceTableRepository: PriceTableRepository,
        userPreferencesRepository: UserPreferencesRepository,
        priceDropNotificationRepository: PriceDropNotificationRepository,
        priceDropAlertHistoryRepository: PriceDropAlertHistoryRepository,
    ): EvaluatePriceDropAlertsUseCase = EvaluatePriceDropAlertsUseCase(
        vehicleRepository = vehicleRepository,
        averagePriceRepository = averagePriceRepository,
        stationPriceRepository = stationPriceRepository,
        priceTableRepository = priceTableRepository,
        userPreferencesRepository = userPreferencesRepository,
        priceDropNotificationRepository = priceDropNotificationRepository,
        priceDropAlertHistoryRepository = priceDropAlertHistoryRepository,
    )

    @Provides
    @Singleton
    fun provideConfigurePriceDropAlertUseCase(
        priceDropNotificationRepository: PriceDropNotificationRepository,
    ): ConfigurePriceDropAlertUseCase = ConfigurePriceDropAlertUseCase(
        priceDropNotificationRepository = priceDropNotificationRepository,
    )

    @Provides
    @Singleton
    fun provideGetCommunityPriceGroupsUseCase(
        flagProvider: CommunityReadsFlagProvider,
        httpGateway: BackendPriceHttpGateway,
        cache: BackendPriceCacheRepository,
    ): GetCommunityPriceGroupsUseCase = GetCommunityPriceGroupsUseCase(
        flagProvider = flagProvider,
        httpGateway = httpGateway,
        cache = cache,
    )

    @Provides
    @Singleton
    fun provideGetServerStationsUseCase(
        flagProvider: CommunityReadsFlagProvider,
        gateway: ServerStationGateway,
        cache: ServerStationCache,
    ): GetServerStationsUseCase = GetServerStationsUseCase(
        flagProvider = flagProvider,
        gateway = gateway,
        cache = cache,
    )

    @Provides
    @Singleton
    fun provideGetServerStationDetailUseCase(
        flagProvider: CommunityReadsFlagProvider,
        gateway: ServerStationGateway,
        cache: ServerStationCache,
    ): GetServerStationDetailUseCase = GetServerStationDetailUseCase(
        flagProvider = flagProvider,
        gateway = gateway,
        cache = cache,
    )

    @Provides
    @Singleton
    fun provideGetNearbyServerStationsUseCase(
        flagProvider: CommunityReadsFlagProvider,
        gateway: ServerStationGateway,
    ): GetNearbyServerStationsUseCase =
        GetNearbyServerStationsUseCase(
            flagProvider = flagProvider,
            gateway = gateway,
        )

    @Provides
    @Singleton
    fun provideAnonymousDeviceFlow(
        flagProvider: AnonymousContributionFlagProvider,
        keys: AnonymousDeviceKeyPort,
    ): AnonymousDeviceFlow = AnonymousDeviceFlow(
        flagProvider = flagProvider,
        keys = keys,
    )

    @Provides
    @Singleton
    fun provideConfirmPriceCaptureUseCase(
        flagProvider: CaptureOcrFlagProvider,
        ocr: OcrPort,
    ): ConfirmPriceCaptureUseCase = ConfirmPriceCaptureUseCase(
        flagProvider = flagProvider,
        ocr = ocr,
    )

    @Provides
    @Singleton
    fun provideEnqueueContributionUseCase(
        flagProvider: ContributionOutboxFlagProvider,
        outbox: ContributionOutboxRepository,
    ): EnqueueContributionUseCase = EnqueueContributionUseCase(
        flagProvider = flagProvider,
        outbox = outbox,
    )

    @Provides
    @Singleton
    fun provideGetOwnedContributionsUseCase(
        outbox: ContributionOutboxRepository,
    ): GetOwnedContributionsUseCase = GetOwnedContributionsUseCase(outbox)

    @Provides
    @Singleton
    fun provideCancelOwnedContributionUseCase(
        outbox: ContributionOutboxRepository,
    ): CancelOwnedContributionUseCase = CancelOwnedContributionUseCase(outbox)

    @Provides
    @Singleton
    fun provideSubmitCommunityVoteUseCase(
        flagProvider: CommunityVoteFlagProvider,
        gateway: CommunityVoteGateway,
    ): SubmitCommunityVoteUseCase = SubmitCommunityVoteUseCase(
        flagProvider = flagProvider,
        gateway = gateway,
    )

    @Provides
    @Singleton
    fun provideSubmitFeedbackUseCase(
        flagProvider: FeedbackFlagProvider,
        gateway: FeedbackGateway,
        outbox: FeedbackOutboxPort,
    ): SubmitFeedbackUseCase = SubmitFeedbackUseCase(
        flagProvider = flagProvider,
        gateway = gateway,
        outbox = outbox,
    )

    @Provides
    @Singleton
    fun provideGetFeedbackPageUseCase(
        flagProvider: FeedbackFlagProvider,
        gateway: FeedbackGateway,
        cache: FeedbackCacheRepository,
    ): GetFeedbackPageUseCase = GetFeedbackPageUseCase(
        flagProvider = flagProvider,
        gateway = gateway,
        cache = cache,
    )
}
