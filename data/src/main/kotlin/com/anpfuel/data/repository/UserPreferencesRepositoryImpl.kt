package com.anpfuel.data.repository

import com.anpfuel.data.local.preferences.UserPreferencesDataStore
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.UserPreferencesRepository
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class UserPreferencesRepositoryImpl @Inject constructor(
    private val userPreferencesDataStore: UserPreferencesDataStore,
) : UserPreferencesRepository {

    override suspend fun getPreferences(): UserPreferences =
        userPreferencesDataStore.read()

    override suspend fun savePreferences(preferences: UserPreferences) {
        userPreferencesDataStore.write(preferences)
    }

    override suspend fun markPublicCityCatalogSynced(expected: UserPreferences, atMillis: Long): Boolean =
        userPreferencesDataStore.markPublicCityCatalogSynced(expected, atMillis)

    override suspend fun clearPublicCityCatalogSyncMarker() = userPreferencesDataStore.clearPublicCityCatalogSyncMarker()

}
