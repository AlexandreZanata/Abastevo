package com.anpfuel.data.di

import com.anpfuel.application.portable.PhotoCache
import com.anpfuel.data.remote.ContributionUploadHttpClient
import com.anpfuel.data.remote.OkHttpClientFactory
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

/**
 * P10-T05 contribution outbox bindings.
 *
 * The base URL is an explicit preview placeholder (RFC 2606 `.invalid`,
 * never resolves): dispatch stays disabled by default and issues nowhere
 * until deployment configuration lands, mirroring the P10-T02 reads and
 * P13 auth precedents. No CameraX, no production URL, no ANP change.
 */
@Module
@InstallIn(SingletonComponent::class)
object ContributionModule {

    /** Preview API base; deployment configuration replaces it. */
    const val PREVIEW_BASE_URL: String = "https://api.anpfuel.example.invalid"

    @Provides
    @Singleton
    fun provideContributionUploadHttpClient(
        photoCache: PhotoCache,
    ): ContributionUploadHttpClient = ContributionUploadHttpClient(
        client = OkHttpClientFactory.create(),
        baseUrl = PREVIEW_BASE_URL,
        photoCache = photoCache,
    )
}
