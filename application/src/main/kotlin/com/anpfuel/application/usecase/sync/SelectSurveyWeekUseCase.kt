package com.anpfuel.application.usecase.sync

import com.anpfuel.domain.event.PreferencesUpdated
import com.anpfuel.domain.event.SurveyWeekSelected
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.domain.valueobject.SurveyWeekSelectionMode

data class SelectSurveyWeekResult(
    val preferences: UserPreferences,
    val surveyWeekSelected: SurveyWeekSelected,
    val preferencesUpdated: PreferencesUpdated,
)

/**
 * UC-009 — Persist user survey week choice and emit domain events (BR-018, BR-019).
 */
class SelectSurveyWeekUseCase(
    private val userPreferencesRepository: UserPreferencesRepository,
    private val eventPublisher: DomainEventPublisher,
) {

    suspend operator fun invoke(
        surveyWeek: SurveyWeek,
        selectionMode: SurveyWeekSelectionMode,
    ): SelectSurveyWeekResult {
        val currentPreferences = userPreferencesRepository.getPreferences()
        val updatedPreferences = currentPreferences.copy(activeSurveyWeek = surveyWeek)
        userPreferencesRepository.savePreferences(updatedPreferences)

        val surveyWeekSelected = SurveyWeekSelected.create(
            payload = SurveyWeekSelected.Payload(
                surveyWeek = surveyWeek,
                selectionMode = selectionMode,
            ),
        )
        val preferencesUpdated = PreferencesUpdated.create(
            payload = PreferencesUpdated.Payload(
                changedKeys = setOf(KEY_ACTIVE_SURVEY_WEEK),
            ),
        )

        eventPublisher.publish(surveyWeekSelected)
        eventPublisher.publish(preferencesUpdated)

        return SelectSurveyWeekResult(
            preferences = updatedPreferences,
            surveyWeekSelected = surveyWeekSelected,
            preferencesUpdated = preferencesUpdated,
        )
    }

    companion object {
        const val KEY_ACTIVE_SURVEY_WEEK = "activeSurveyWeek"
    }
}
