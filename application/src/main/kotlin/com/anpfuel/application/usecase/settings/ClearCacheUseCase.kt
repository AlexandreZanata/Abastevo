package com.anpfuel.application.usecase.settings

import com.anpfuel.domain.event.CacheClearScope
import com.anpfuel.domain.event.CacheCleared
import com.anpfuel.domain.repository.CacheRepository
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.StationPriceRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.StationDetailRetentionRule

data class ClearCacheResult(
    val scope: CacheClearScope,
    val event: CacheCleared,
)

class ClearCacheUseCase(
    private val cacheRepository: CacheRepository,
    private val stationPriceRepository: StationPriceRepository,
    private val userPreferencesRepository: UserPreferencesRepository,
    private val eventPublisher: DomainEventPublisher,
) {

    suspend operator fun invoke(scope: CacheClearScope): ClearCacheResult {
        when (scope) {
            CacheClearScope.ALL -> clearAllImportedData()
            CacheClearScope.STATION_DETAIL_ONLY -> applyStationDetailRetention()
        }

        val event = CacheCleared.create(
            payload = CacheCleared.Payload(scope = scope),
        )
        eventPublisher.publish(event)

        return ClearCacheResult(
            scope = scope,
            event = event,
        )
    }

    private suspend fun clearAllImportedData() {
        cacheRepository.clearAllImportedData()
        val preferences = userPreferencesRepository.getPreferences()
        userPreferencesRepository.savePreferences(
            preferences.copy(
                preferredState = null,
                preferredMunicipality = null,
                onboardingCompleted = false,
                locationPromptCompleted = false,
            ),
        )
    }

    private suspend fun applyStationDetailRetention() {
        val retentionWeeks = StationDetailRetentionRule.retentionWeeks(
            userPreferencesRepository.getPreferences(),
        )
        stationPriceRepository.deleteStationPricesOlderThanRetention(retentionWeeks)
    }
}
