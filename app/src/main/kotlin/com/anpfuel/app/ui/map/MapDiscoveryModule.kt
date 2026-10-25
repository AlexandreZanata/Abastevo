package com.anpfuel.app.ui.map

import com.anpfuel.data.repository.ChargingDiscoveryRepository
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

@Module
@InstallIn(SingletonComponent::class)
object MapDiscoveryModule {
    @Provides
    @Singleton
    fun provideChargingMarkersSource(
        repo: ChargingDiscoveryRepository,
    ): ChargingMarkersSource = ChargingMarkersSource { limit ->
        try {
            repo.list(
                ChargingDiscoveryRepository.OCM_PROFILE,
                null,
                null,
                null,
                null,
                limit.coerceIn(1, 100),
                0,
            ).map { it.place }
        } catch (_: Exception) {
            emptyList()
        }
    }
}
