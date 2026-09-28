package com.anpfuel.data.repository

import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.UserPreferencesRepository

internal class InMemoryUserPreferencesRepository(
    private var preferences: UserPreferences = UserPreferences(),
) : UserPreferencesRepository {

    override suspend fun getPreferences(): UserPreferences = preferences

    override suspend fun savePreferences(preferences: UserPreferences) {
        this.preferences = preferences
    }
}
