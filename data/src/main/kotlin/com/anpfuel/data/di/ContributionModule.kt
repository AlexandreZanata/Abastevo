package com.anpfuel.data.di

import com.anpfuel.application.portable.PhotoCache
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.ContributionUploadHttpClient
import com.anpfuel.data.remote.OkHttpClientFactory
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton

/**
 * P10-T05 contribution outbox bindings (origin shared in P34-T02).
 *
 * DI uses the shared [ApiEnvironment] staging selection. No CameraX,
 * no production URL, no ANP change.
 */
@Module
@InstallIn(SingletonComponent::class)
object ContributionModule {

    /** Preview API base; kept for reference, DI uses the shared origin. */
    const val PREVIEW_BASE_URL: String = "https://api.anpfuel.example.invalid"

    @Provides
    @Singleton
    fun provideContributionUploadHttpClient(
        photoCache: PhotoCache,
        @Named("apiOrigin") environment: ApiEnvironment,
    ): ContributionUploadHttpClient = ContributionUploadHttpClient(
        client = OkHttpClientFactory.create(),
        baseUrl = environment.origin,
        photoCache = photoCache,
    )
}
