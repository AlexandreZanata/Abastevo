package com.anpfuel.data.di

import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.FeedbackHttpClient
import com.anpfuel.data.remote.OkHttpClientFactory
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton

/**
 * P17-T02 feedback transport bindings (origin shared in P34-T02).
 *
 * DI uses the shared [ApiEnvironment] staging selection; writes stay
 * queued/offline and reads stay cache-gated until flags enable them,
 * mirroring the P10/P13 precedent.
 */
@Module
@InstallIn(SingletonComponent::class)
object FeedbackModule {

    @Provides
    @Singleton
    fun provideFeedbackHttpClient(
        sessions: AuthSessionStore,
        auth: AuthFlow,
        @Named("apiOrigin") environment: ApiEnvironment,
    ): FeedbackHttpClient =
        FeedbackHttpClient(
            client = OkHttpClientFactory.create(),
            baseUrl = environment.origin,
            sessions = sessions,
            auth = auth,
        )
}
