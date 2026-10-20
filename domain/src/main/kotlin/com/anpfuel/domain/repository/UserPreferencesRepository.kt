package com.anpfuel.domain.repository

import com.anpfuel.domain.model.UserPreferences

/**
 * Port for local user preferences (UC-003, UC-008, BR-012).
 */
interface UserPreferencesRepository {

    suspend fun getPreferences(): UserPreferences

    suspend fun savePreferences(preferences: UserPreferences)

    /** Implementations with durable preferences must override with an atomic conditional update. */
    suspend fun markPublicCityCatalogSynced(expected: UserPreferences, atMillis: Long): Boolean {
        val current = getPreferences()
        if (!current.syncPublicCityCatalog || current.publicCityCatalogGeneration != expected.publicCityCatalogGeneration ||
            current.preferredState != expected.preferredState || current.preferredMunicipality != expected.preferredMunicipality ||
            current.autoSyncOnWifi != expected.autoSyncOnWifi) return false
        savePreferences(current.copy(publicCityCatalogLastSyncedAtMillis = atMillis))
        return true
    }

    suspend fun clearPublicCityCatalogSyncMarker() {
        val current = getPreferences()
        savePreferences(current.copy(publicCityCatalogLastSyncedAtMillis = null, publicCityCatalogGeneration = current.publicCityCatalogGeneration + 1))
    }

}
