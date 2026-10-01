package com.anpfuel.data.di

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.remote.FeedbackHttpClient
import com.anpfuel.data.remote.OkHttpClientFactory
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

/**
 * P17-T02 feedback transport bindings.
 *
 * The base URL is the same explicit preview placeholder as the
 * community module (RFC 2606 `.invalid`, never resolves): writes
 * stay queued/offline and reads stay cache-gated until deployment
 * configuration lands, mirroring the P10/P13 precedent.
 */
@Module
@InstallIn(SingletonComponent::class)
object FeedbackModule {

    @Provides
    @Singleton
    fun provideFeedbackHttpClient(sessions: AuthSessionStore): FeedbackHttpClient =
        FeedbackHttpClient(
            client = OkHttpClientFactory.create(),
            baseUrl = CommunityModule.PREVIEW_BASE_URL,
            sessions = sessions,
        )
}
