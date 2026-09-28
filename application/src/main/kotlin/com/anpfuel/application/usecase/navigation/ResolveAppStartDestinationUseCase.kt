package com.anpfuel.application.usecase.navigation

import com.anpfuel.domain.model.AppStartDestination
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.AppStartDestinationRule

/**
 * UC-009 — Resolve cold-start navigation destination (Phase 12.4.5).
 */
class ResolveAppStartDestinationUseCase(
    private val userPreferencesRepository: UserPreferencesRepository,
    private val priceTableRepository: PriceTableRepository,
) {

    suspend operator fun invoke(): AppStartDestination {
        val preferences = userPreferencesRepository.getPreferences()
        val importedSurveys = priceTableRepository.getImportedPriceSurveys()

        return AppStartDestinationRule.resolve(
            onboardingCompleted = preferences.onboardingCompleted,
            activeSurveyWeek = preferences.activeSurveyWeek,
            importedSurveys = importedSurveys,
            autoDownloadLatestWeek = preferences.autoDownloadLatestWeek,
        )
    }
}
