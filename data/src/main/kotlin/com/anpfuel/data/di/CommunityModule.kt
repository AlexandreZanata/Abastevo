package com.anpfuel.data.di

import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.BackendStationPriceHttpClient
import com.anpfuel.data.remote.CommunityVoteHttpClient
import com.anpfuel.data.remote.OkHttpClientFactory
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton

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
