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
    private val directory: GetServerStationsUseCase? = null,
) {
    suspend operator fun invoke() {
        if (directory == null) cache.clearPublicPages() else directory.clearPublicPages()
        userPreferencesRepository.clearPublicCityCatalogSyncMarker()
    }
}
