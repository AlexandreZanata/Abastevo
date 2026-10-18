package com.anpfuel.application.usecase.directory

import com.anpfuel.domain.repository.ServerStationCache
import com.anpfuel.domain.repository.UserPreferencesRepository

/**
 * Public catalog management: drops stored directory pages (every
 * scope) while keeping individually fetched details, and resets the
 * last-sync marker since its pages are gone. ANP tables are untouched.
 */
class ClearPublicCatalogUseCase(
    private val cache: ServerStationCache,
    private val userPreferencesRepository: UserPreferencesRepository,
) {
    suspend operator fun invoke() {
        cache.clearPublicPages()
        val preferences = userPreferencesRepository.getPreferences()
        if (preferences.publicCityCatalogLastSyncedAtMillis != null) {
            userPreferencesRepository.savePreferences(
                preferences.copy(publicCityCatalogLastSyncedAtMillis = null),
            )
        }
    }
}
