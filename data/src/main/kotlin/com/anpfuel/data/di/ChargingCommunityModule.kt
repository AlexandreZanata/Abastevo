package com.anpfuel.data.di

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.charging.ChargingCommunityUseCase
import com.anpfuel.data.local.preferences.ChargingPendingStoreImpl
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.OkHttpClientFactory
import com.anpfuel.data.remote.charging.ChargingObservationHttpClient
import com.anpfuel.domain.charging.ChargingObservationGateway
import com.anpfuel.domain.charging.ChargingPendingStore
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton

/** EE-22 community EV transport and durable pending operations. */
@Module
@InstallIn(SingletonComponent::class)
object ChargingCommunityModule {

    @Provides
    @Singleton
    fun provideChargingObservationClient(
        @Named("apiOrigin") environment: ApiEnvironment,
    ): ChargingObservationHttpClient =
        ChargingObservationHttpClient(OkHttpClientFactory.create(maxRetries = 0), environment.origin)

    @Provides
    @Singleton
    fun provideChargingObservationGateway(client: ChargingObservationHttpClient): ChargingObservationGateway = client

    @Provides
    @Singleton
    fun provideChargingPendingStore(impl: ChargingPendingStoreImpl): ChargingPendingStore = impl

    @Provides
    @Singleton
    fun provideChargingCommunityUseCase(
        gateway: ChargingObservationGateway,
        pending: ChargingPendingStore,
        sessions: AuthSessionStore,
        @Named("apiOrigin") environment: ApiEnvironment,
    ): ChargingCommunityUseCase = ChargingCommunityUseCase(gateway, pending, sessions, environment.origin)
}
