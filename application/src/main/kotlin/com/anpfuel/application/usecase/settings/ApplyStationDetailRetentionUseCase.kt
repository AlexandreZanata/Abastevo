package com.anpfuel.application.usecase.settings

import com.anpfuel.domain.repository.StationPriceRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.StationDetailRetentionRule

/**
 * UC-008 / BR-013 — Applies station detail retention after a successful station import.
 */
class ApplyStationDetailRetentionUseCase(
    private val userPreferencesRepository: UserPreferencesRepository,
    private val stationPriceRepository: StationPriceRepository,
) {

    suspend operator fun invoke() {
        val retentionWeeks = StationDetailRetentionRule.retentionWeeks(
            userPreferencesRepository.getPreferences(),
        )
        stationPriceRepository.deleteStationPricesOlderThanRetention(retentionWeeks)
    }
}
