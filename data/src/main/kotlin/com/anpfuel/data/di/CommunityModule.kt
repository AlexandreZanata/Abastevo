package com.anpfuel.data.di

import com.anpfuel.data.remote.BackendStationPriceHttpClient
import com.anpfuel.data.remote.OkHttpClientFactory
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

/**
 * P10-T02 backend reads bindings.
 *
 * The base URL is an explicit preview placeholder (RFC 2606 `.invalid`,
 * never resolves): reads stay disabled by default and issue nowhere
 * until deployment configuration lands, mirroring the P13 auth precedent.
 */
@Module
@InstallIn(SingletonComponent::class)
object CommunityModule {

    /** Preview API base; deployment configuration replaces it. */
    const val PREVIEW_BASE_URL: String = "https://api.anpfuel.example.invalid"

    @Provides
    @Singleton
    fun provideBackendStationPriceHttpClient(): BackendStationPriceHttpClient =
        BackendStationPriceHttpClient(
            client = OkHttpClientFactory.create(),
            baseUrl = PREVIEW_BASE_URL,
        )
}
