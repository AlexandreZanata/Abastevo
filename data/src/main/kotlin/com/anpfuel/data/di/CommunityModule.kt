package com.anpfuel.data.di

import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.BackendStationPriceHttpClient
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
    fun provideCityFeedReader(@Named("apiOrigin") environment: ApiEnvironment): CityCommunityFeedReader =
        CityCommunityFeedHttpClient(
            OkHttpClientFactory.create(maxRetries = 0).newBuilder()
                .callTimeout(12, TimeUnit.SECONDS)
                .followRedirects(false)
                .followSslRedirects(false)
                .build(),
            environment.origin,
        )

    @Provides
    fun provideCityFeedUseCase(reader: CityCommunityFeedReader, locations: SelectLocationUseCase, catalog: MunicipalityCatalogRepository) =
        GetCityCommunityFeedUseCase(reader, locations, catalog)

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
}
