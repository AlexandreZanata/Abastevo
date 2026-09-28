package com.anpfuel.application.usecase.onboarding

import com.anpfuel.application.usecase.sync.SyncPriceTablesResult
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.OnboardingCompletionRule

sealed class CompleteOnboardingResult {
    data object Completed : CompleteOnboardingResult()
    data object AlreadyCompleted : CompleteOnboardingResult()
    data object NotReady : CompleteOnboardingResult()
    data object Skipped : CompleteOnboardingResult()
}

class CompleteOnboardingUseCase(
    private val userPreferencesRepository: UserPreferencesRepository,
    private val priceTableRepository: PriceTableRepository,
) {

    suspend fun completeAfterSync(syncResult: SyncPriceTablesResult): CompleteOnboardingResult {
        val preferences = userPreferencesRepository.getPreferences()
        if (preferences.onboardingCompleted) {
            return CompleteOnboardingResult.AlreadyCompleted
        }

        val hasImportedSummary = hasImportedSummaryData()
        if (!OnboardingCompletionRule.canMarkComplete(syncResult.outcome, hasImportedSummary)) {
            return CompleteOnboardingResult.NotReady
        }

        persistOnboardingCompleted(preferences)
        return CompleteOnboardingResult.Completed
    }

    suspend fun skipSync(): CompleteOnboardingResult {
        val preferences = userPreferencesRepository.getPreferences()
        if (preferences.onboardingCompleted) {
            return CompleteOnboardingResult.AlreadyCompleted
        }

        return CompleteOnboardingResult.Skipped
    }

    private suspend fun hasImportedSummaryData(): Boolean =
        priceTableRepository.getImportedPriceSurveys().any { it.hasSummaryData }

    private suspend fun persistOnboardingCompleted(preferences: UserPreferences) {
        userPreferencesRepository.savePreferences(
            preferences.copy(onboardingCompleted = true),
        )
    }
}
