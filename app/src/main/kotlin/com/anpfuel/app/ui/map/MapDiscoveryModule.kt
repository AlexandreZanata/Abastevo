package com.anpfuel.app.ui.map

import com.anpfuel.data.repository.ChargingDiscoveryRepository
import com.anpfuel.domain.charging.ChargingPlace
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton
import kotlinx.coroutines.CancellationException

@Module
@InstallIn(SingletonComponent::class)
object MapDiscoveryModule {
    @Provides
    @Singleton
    fun provideChargingMarkersSource(
        repo: ChargingDiscoveryRepository,
    ): ChargingMarkersSource = object : ChargingMarkersSource {
        override suspend fun list(limit: Int): List<ChargingPlace> =
            query(ChargingMarkersQuery(limit = limit)).places

        override suspend fun query(request: ChargingMarkersQuery): ChargingMarkersResult {
            try {
                val meta = repo.generationMeta(ChargingDiscoveryRepository.OCM_PROFILE)
                    ?: return ChargingMarkersResult(
                        places = emptyList(),
                        snapshotPresent = false,
                        error = null,
                    )
                val targetLimit = request.limit.coerceIn(1, 200)
                val maxScan = meta.placeCount.toInt().coerceIn(ChargingDiscoveryRepository.PAGE_SIZE, 10_000)
                val matched = ArrayList<ChargingPlace>(targetLimit)
                var offset = 0
                while (offset < maxScan && matched.size < targetLimit) {
                    val page = repo.list(
                        profile = ChargingDiscoveryRepository.OCM_PROFILE,
                        generation = meta.generation,
                        connector = null,
                        current = null,
                        access = null,
                        limit = ChargingDiscoveryRepository.PAGE_SIZE,
                        offset = offset,
                    )
                    if (page.isEmpty()) break
                    for (row in page) {
                        if (matchesChargingQuery(row.place, request)) {
                            matched.add(row.place)
                            if (matched.size >= targetLimit) break
                        }
                    }
                    if (page.size < ChargingDiscoveryRepository.PAGE_SIZE) break
                    offset += ChargingDiscoveryRepository.PAGE_SIZE
                }
                return ChargingMarkersResult(
                    places = matched,
                    snapshotPresent = true,
                    error = null,
                )
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                return ChargingMarkersResult(
                    places = emptyList(),
                    snapshotPresent = false,
                    error = e.message ?: "charging-snapshot-failed",
                )
            }
        }
    }
}
