package com.anpfuel.application.usecase.onboarding

import com.anpfuel.domain.event.PreferencesUpdated
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.UserPreferencesRepository

/**
 * UC-012 — Marks the optional onboarding location prompt as completed so it is not shown again.
 */
class CompleteLocationPromptUseCase(
    private val userPreferencesRepository: UserPreferencesRepository,
    private val eventPublisher: DomainEventPublisher,
) {

    suspend operator fun invoke() {
        val current = userPreferencesRepository.getPreferences()
        if (current.locationPromptCompleted) {
            return
        }

        val updated = current.copy(locationPromptCompleted = true)
        userPreferencesRepository.savePreferences(updated)

        eventPublisher.publish(
            PreferencesUpdated.create(
                payload = PreferencesUpdated.Payload(
                    changedKeys = setOf(KEY_LOCATION_PROMPT_COMPLETED),
                ),
            ),
        )
    }

    companion object {
        const val KEY_LOCATION_PROMPT_COMPLETED = "locationPromptCompleted"
    }
}
