package com.anpfuel.application.usecase.directory

import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.UserPreferencesRepository
import java.io.IOException
import kotlinx.coroutines.withTimeoutOrNull

/** Five pages / 2.5 MiB / 45 seconds per batch; partial staging resumes on the next run. */
class SyncPublicCatalogUseCase(
    private val getServerStations: GetServerStationsUseCase,
    private val userPreferencesRepository: UserPreferencesRepository,
) {
    data class SyncResult(
        val pagesFetched: Int,
        val complete: Boolean,
        val outcome: ServerStationsOutcome,
        val preferences: UserPreferences? = null,
    )

    suspend fun sync(): SyncResult {
        val initial = userPreferencesRepository.getPreferences()
        if (!initial.syncPublicCityCatalog) return SyncResult(0, false, ServerStationsOutcome.Disabled)
        val result = withTimeoutOrNull(MAX_RUN_MILLIS) {
            getServerStations.syncBatch(PAGE_SIZE, MAX_PAGES) {
                val current = userPreferencesRepository.getPreferences()
                current.syncPublicCityCatalog && current.publicCityCatalogGeneration == initial.publicCityCatalogGeneration &&
                    current.preferredState == initial.preferredState && current.preferredMunicipality == initial.preferredMunicipality &&
                    current.autoSyncOnWifi == initial.autoSyncOnWifi
            }
        } ?: SyncResult(0, false, ServerStationsOutcome.Unavailable(IOException("directory sync time budget exceeded")))
        return result.copy(preferences = initial)
    }

    companion object {
        const val PAGE_SIZE = 20
        const val MAX_PAGES = 5
        const val MAX_RUN_MILLIS = 45_000L
    }
}
