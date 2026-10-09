package com.anpfuel.data.di

import com.anpfuel.data.remote.AnonymousProofHttpClient
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.OkHttpClientFactory
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton

/**
 * P10-T03 anonymous-proof bindings (origin shared in P34-T02).
 *
 * DI uses the shared [ApiEnvironment] staging selection; preview
 * (RFC 2606 `.invalid`, never resolves) stays available as
 * [ApiEnvironment.PREVIEW]. Private key material never crosses the HTTP client.
 */
@Module
@InstallIn(SingletonComponent::class)
object AnonymousModule {

    /** Preview API base; kept for reference, DI uses the shared origin. */
    const val PREVIEW_BASE_URL: String = "https://api.anpfuel.example.invalid"

    @Provides
    @Singleton
    fun provideAnonymousProofHttpClient(
        @Named("apiOrigin") environment: ApiEnvironment,
    ): AnonymousProofHttpClient =
        AnonymousProofHttpClient(
            client = OkHttpClientFactory.create(maxRetries = 0).newBuilder()
                .followRedirects(false).followSslRedirects(false).build(),
            baseUrl = environment.origin,
        )
}
