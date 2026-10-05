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
    fun profileRead(gateway: StationProfileGateway) = GetStationProfileUseCase(gateway)
}
