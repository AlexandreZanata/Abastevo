package com.anpfuel.data.di

import com.anpfuel.application.portable.PhotoCache
import com.anpfuel.application.port.ContributionScopeProvider
import com.anpfuel.domain.model.ContributionScope
import com.anpfuel.data.remote.PhotoProofTransport
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.ContributionUploadHttpClient
import com.anpfuel.data.remote.OkHttpClientFactory
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import com.anpfuel.data.local.dao.PhotoUploadSessionDao
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

    @Provides
    @Singleton
    fun provideContributionScopeProvider(proof: PhotoProofTransport): ContributionScopeProvider =
        ContributionScopeProvider { ContributionScope(proof.localScope(), proof.origin) }

    /** Preview API base; kept for reference, DI uses the shared origin. */
    const val PREVIEW_BASE_URL: String = "https://api.anpfuel.example.invalid"

    @Provides
    @Singleton
    fun provideContributionUploadHttpClient(
        photoCache: PhotoCache,
        proof: PhotoProofTransport,
        sessions: PhotoUploadSessionDao,
    ): ContributionUploadHttpClient = ContributionUploadHttpClient(
        client = OkHttpClientFactory.create(maxRetries = 0),
        proof = proof,
        sessions = sessions,
        photoCache = photoCache,
    )
}
