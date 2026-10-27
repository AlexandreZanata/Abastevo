package com.anpfuel.data.di

import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.OkHttpClientFactory
import com.anpfuel.data.remote.contributor.ContributorsHttpClient
import com.anpfuel.domain.repository.ContributorsGateway
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton

/** B-BR-CTB public contributors transport. */
@Module
@InstallIn(SingletonComponent::class)
object ContributorsModule {

    @Provides
    @Singleton
    fun provideContributorsClient(
        @Named("apiOrigin") environment: ApiEnvironment,
    ): ContributorsHttpClient =
        ContributorsHttpClient(OkHttpClientFactory.create(maxRetries = 0), environment.origin)

    @Provides
    @Singleton
    fun provideContributorsGateway(client: ContributorsHttpClient): ContributorsGateway = client
}
