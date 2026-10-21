package com.anpfuel.data.di

import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.BackendStationPriceHttpClient
import com.anpfuel.data.remote.CommunityHelpfulnessHttpClient
import com.anpfuel.data.remote.TrainingConsentHttpClient
import com.anpfuel.data.remote.CommunityVoteHttpClient
import com.anpfuel.data.remote.OkHttpClientFactory
import com.anpfuel.data.remote.CityCommunityFeedHttpClient
import com.anpfuel.domain.community.CityCommunityFeedReader
import com.anpfuel.application.usecase.community.GetCityCommunityFeedUseCase
import com.anpfuel.application.usecase.location.SelectLocationUseCase
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton
import java.util.concurrent.TimeUnit

/**
 * P10-T02 backend reads bindings (origin shared in P34-T02).
 *
 * DI uses the shared [ApiEnvironment] staging selection; preview
 * (RFC 2606 `.invalid`, never resolves) stays available as
 * [ApiEnvironment.PREVIEW].
 */
@Module
@InstallIn(SingletonComponent::class)
object CommunityModule {

    @Provides
    @Singleton
    fun provideCommunityFeedCacheStore(
        @Named("apiOrigin") environment: ApiEnvironment,
        dao: com.anpfuel.data.local.dao.CommunityFeedCacheDao,
    ): com.anpfuel.data.repository.CommunityFeedCacheStore {
        val client = CityCommunityFeedHttpClient(
            OkHttpClientFactory.create(maxRetries = 0).newBuilder()
                .callTimeout(12, TimeUnit.SECONDS)
                .followRedirects(false)
                .followSslRedirects(false)
                .build(),
            environment.origin,
        )
        return com.anpfuel.data.repository.CommunityFeedCacheStore(
            networkFeed = client,
            networkSummary = client,
            dao = dao,
            origin = environment.origin,
        )
    }

    @Provides
    fun provideCityFeedReader(store: com.anpfuel.data.repository.CommunityFeedCacheStore): CityCommunityFeedReader = store

    @Provides
    fun provideCityFeedSnapshot(store: com.anpfuel.data.repository.CommunityFeedCacheStore): com.anpfuel.domain.community.CommunityFeedLocalSnapshot = store

    @Provides
    fun provideCityFeedUseCase(reader: CityCommunityFeedReader, locations: SelectLocationUseCase, catalog: MunicipalityCatalogRepository) =
        GetCityCommunityFeedUseCase(reader, locations, catalog)

    @Provides
    fun provideCitySummaryReader(store: com.anpfuel.data.repository.CommunityFeedCacheStore): com.anpfuel.domain.community.CityCommunitySummaryReader = store

    @Provides
    fun provideCitySummaryUseCase(reader: com.anpfuel.domain.community.CityCommunitySummaryReader, cityFeed: GetCityCommunityFeedUseCase) =
        com.anpfuel.application.usecase.community.GetCityCommunitySummaryUseCase(reader, cityFeed)

    /** Preview API base; kept for reference, DI uses the shared origin. */
    const val PREVIEW_BASE_URL: String = "https://api.anpfuel.example.invalid"

    @Provides
    @Singleton
    fun provideBackendStationPriceHttpClient(
        @Named("apiOrigin") environment: ApiEnvironment,
    ): BackendStationPriceHttpClient =
        BackendStationPriceHttpClient(
            client = OkHttpClientFactory.create(),
            baseUrl = environment.origin,
        )

    @Provides
    @Singleton
    fun provideCommunityVoteHttpClient(
        @Named("apiOrigin") environment: ApiEnvironment,
    ): CommunityVoteHttpClient =
        CommunityVoteHttpClient(
            client = OkHttpClientFactory.create(),
            baseUrl = environment.origin,
        )

    @Provides
    @Singleton
    fun provideCommunityHelpfulnessHttpClient(
        proof: com.anpfuel.data.remote.PhotoProofTransport,
        @Named("apiOrigin") environment: ApiEnvironment,
    ): CommunityHelpfulnessHttpClient =
        CommunityHelpfulnessHttpClient(
            proof = proof,
            client = OkHttpClientFactory.create(),
            baseUrl = environment.origin,
        )

    @Provides
    @Singleton
    fun provideTrainingConsentHttpClient(
        proof: com.anpfuel.data.remote.PhotoProofTransport,
    ): TrainingConsentHttpClient = TrainingConsentHttpClient(proof)

}
