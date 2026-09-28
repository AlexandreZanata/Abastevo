package com.anpfuel.application.usecase.settings

import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.UserPreferencesRepository

class GetSettingsUseCase(
    private val userPreferencesRepository: UserPreferencesRepository,
) {

    suspend operator fun invoke(): UserPreferences =
        userPreferencesRepository.getPreferences()
}
