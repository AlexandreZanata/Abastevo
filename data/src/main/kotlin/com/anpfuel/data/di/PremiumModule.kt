package com.anpfuel.data.di

import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.OkHttpClientFactory
import com.anpfuel.data.remote.premium.PremiumStatusHttpClient
import com.anpfuel.domain.repository.PremiumStatusGateway
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton

/** MEX-16 verified Premium status transport. */
@Module
@InstallIn(SingletonComponent::class)
object PremiumModule {

    @Provides
    @Singleton
    fun providePremiumStatusClient(
        @Named("apiOrigin") environment: ApiEnvironment,
    ): PremiumStatusHttpClient =
        PremiumStatusHttpClient(OkHttpClientFactory.create(maxRetries = 0), environment.origin)

    @Provides
    @Singleton
    fun providePremiumStatusGateway(client: PremiumStatusHttpClient): PremiumStatusGateway = client
}
