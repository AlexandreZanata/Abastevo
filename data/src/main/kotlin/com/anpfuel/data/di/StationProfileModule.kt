package com.anpfuel.data.di

import com.anpfuel.application.usecase.profile.GetStationProfileUseCase
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.OkHttpClientFactory
import com.anpfuel.data.remote.profile.StationProfileHttpClient
import com.anpfuel.domain.repository.StationProfileGateway
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton

@Module
@InstallIn(SingletonComponent::class)
object StationProfileModule {
    @Provides
    @Singleton
    fun profileGateway(@Named("apiOrigin") environment: ApiEnvironment): StationProfileGateway =
        StationProfileHttpClient(OkHttpClientFactory.create(maxRetries = 0), environment.origin)

    @Provides
    @Singleton
    fun claimGateway(@Named("apiOrigin") environment: ApiEnvironment): com.anpfuel.application.usecase.profile.ProfileClaimGateway =
        com.anpfuel.data.remote.profile.ProfileClaimHttpClient(
            OkHttpClientFactory.create(maxRetries = 0).newBuilder().retryOnConnectionFailure(false).build(), environment.origin,
        )

    @Provides
    fun profileActions(
        gateway: com.anpfuel.application.usecase.profile.ProfileClaimGateway,
        sessions: com.anpfuel.application.portable.AuthSessionStore,
    ) = com.anpfuel.application.usecase.profile.StationProfileActions(gateway, sessions) { System.currentTimeMillis() / 1000L }

    @Provides
    fun profileRead(gateway: StationProfileGateway) = GetStationProfileUseCase(gateway)
}
