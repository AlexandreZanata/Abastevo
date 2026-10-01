package com.anpfuel.data.di

import com.anpfuel.data.remote.AnonymousProofHttpClient
import com.anpfuel.data.remote.OkHttpClientFactory
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

/**
 * P10-T03 anonymous-proof bindings.
 *
 * The base URL is an explicit preview placeholder (RFC 2606 `.invalid`,
 * never resolves): proofs stay disabled by default and issue nowhere
 * until deployment configuration lands, mirroring the P10-T02/P13
 * precedent. Private key material never crosses the HTTP client.
 */
@Module
@InstallIn(SingletonComponent::class)
object AnonymousModule {

    /** Preview API base; deployment configuration replaces it. */
    const val PREVIEW_BASE_URL: String = "https://api.anpfuel.example.invalid"

    @Provides
    @Singleton
    fun provideAnonymousProofHttpClient(): AnonymousProofHttpClient =
        AnonymousProofHttpClient(
            client = OkHttpClientFactory.create(),
            baseUrl = PREVIEW_BASE_URL,
        )
}
