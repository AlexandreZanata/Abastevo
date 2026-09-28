package com.anpfuel.data.repository

import com.anpfuel.data.local.preferences.GeocodeCacheStore
import com.anpfuel.data.mapper.NominatimResponseMapper
import com.anpfuel.data.remote.GeocodeCacheKeyFormatter
import com.anpfuel.data.remote.NominatimRateLimiter
import com.anpfuel.data.remote.NominatimReverseGeocodeClient
import com.anpfuel.domain.model.ReverseGeocodeResult
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.repository.ReverseGeocodeOutcome
import com.anpfuel.domain.repository.ReverseGeocodeRepository
import com.anpfuel.domain.valueobject.DeviceLocation
import java.io.IOException
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

@Singleton
class ReverseGeocodeRepositoryImpl @Inject constructor(
    private val geocodeCacheStore: GeocodeCacheStore,
    private val nominatimClient: NominatimReverseGeocodeClient,
    private val rateLimiter: NominatimRateLimiter,
    private val municipalityCatalogRepository: MunicipalityCatalogRepository,
) : ReverseGeocodeRepository {

    private val requestMutex = Mutex()

    override suspend fun reverseGeocode(location: DeviceLocation): ReverseGeocodeOutcome {
        val cacheKey = GeocodeCacheKeyFormatter.format(location)
        geocodeCacheStore.get(cacheKey)?.let { cached ->
            return validateCatalog(cached)
        }

        return requestMutex.withLock {
            geocodeCacheStore.get(cacheKey)?.let { cached ->
                return@withLock validateCatalog(cached)
            }

            if (!rateLimiter.canRequest()) {
                return@withLock ReverseGeocodeOutcome.RateLimited
            }

            val parsed = try {
                rateLimiter.recordRequest()
                nominatimClient.reverseGeocode(location)
            } catch (_: IOException) {
                return@withLock ReverseGeocodeOutcome.NetworkError
            }

            if (parsed == null) {
                return@withLock ReverseGeocodeOutcome.InvalidResponse
            }

            val preliminary = ReverseGeocodeResult(
                state = parsed.state,
                municipality = parsed.municipality,
                displayName = parsed.displayName,
            )

            when (val validated = validateCatalog(preliminary)) {
                is ReverseGeocodeOutcome.Success -> {
                    geocodeCacheStore.put(cacheKey, validated.result)
                    validated
                }
                else -> validated
            }
        }
    }

    private suspend fun validateCatalog(result: ReverseGeocodeResult): ReverseGeocodeOutcome {
        val catalogEntry = municipalityCatalogRepository.findCatalogEntry(
            state = result.state,
            municipality = result.municipality,
        ) ?: return ReverseGeocodeOutcome.MunicipalityNotInCatalog

        return ReverseGeocodeOutcome.Success(
            result = result.copy(
                municipality = catalogEntry.municipality,
            ),
        )
    }
}
