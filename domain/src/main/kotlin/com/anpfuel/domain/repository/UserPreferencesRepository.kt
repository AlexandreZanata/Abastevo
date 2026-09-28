package com.anpfuel.domain.repository

import com.anpfuel.domain.model.UserPreferences

/**
 * Port for local user preferences (UC-003, UC-008, BR-012).
 */
interface UserPreferencesRepository {

    suspend fun getPreferences(): UserPreferences

    suspend fun savePreferences(preferences: UserPreferences)
}
