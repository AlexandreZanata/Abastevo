package com.anpfuel.data.di

import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.OkHttpClientFactory
import com.anpfuel.data.remote.charging.ChargingSnapshotHttpClient
import com.anpfuel.data.repository.ChargingDiscoveryRepository
import com.anpfuel.data.repository.RoomEvCandidateSource
import com.anpfuel.application.usecase.ev.PlanEvTripUseCase
import com.anpfuel.domain.ev.EvCandidateSource
import com.anpfuel.domain.ev.EvRoadRouter
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton

/**
 * EE-21 public charging snapshot bindings (EE-09 reads, no-store).
 *
 * The client shares the staging origin; no response cache exists —
 * Room generations are the only store.
 */
@Module
@InstallIn(SingletonComponent::class)
object ChargingModule {

    @Provides
    @Singleton
    fun provideChargingSnapshotHttpClient(
        @Named("apiOrigin") environment: ApiEnvironment,
    ): ChargingSnapshotHttpClient =
        ChargingSnapshotHttpClient(OkHttpClientFactory.create(maxRetries = 0), environment.origin)

    @Provides
    @Singleton
    fun provideEvCandidateSource(repository: ChargingDiscoveryRepository): EvCandidateSource =
        RoomEvCandidateSource(repository)

    @Provides
    @Singleton
    fun providePlanEvTripUseCase(router: EvRoadRouter, candidates: EvCandidateSource): PlanEvTripUseCase =
        PlanEvTripUseCase(router, candidates)
}
