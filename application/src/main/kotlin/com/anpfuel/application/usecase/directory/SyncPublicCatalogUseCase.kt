package com.anpfuel.application.usecase.directory

import com.anpfuel.domain.repository.UserPreferencesRepository
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive

/**
 * Bounded public-catalog traversal for background sync.
 *
 * Pages the city directory through the same user-initiated path
 * (first page, then continuations), revalidating the opt-in before
 * every page so disabling stops the traversal without marking
 * anything complete. Never downloads station details, never invents
 * deltas: continuation state is the server cursor only.
 *
 * Documented limits: [PAGE_SIZE] items per request (server max 100),
 * [MAX_PAGES] pages per run, one 512 KiB response cap per page
 * (transport), 15-minute sealed cursors (server). A run that stops
 * early — opt-out, cancellation, transport failure, or the page cap
 * with pages remaining — is partial: cached pages stay for offline
 * use but callers must not stamp it complete.
 */
class SyncPublicCatalogUseCase(
    private val getServerStations: GetServerStationsUseCase,
    private val userPreferencesRepository: UserPreferencesRepository,
) {
    data class SyncResult(
        val pagesFetched: Int,
        val complete: Boolean,
        val outcome: ServerStationsOutcome,
    )

    suspend fun sync(): SyncResult {
        if (!userPreferencesRepository.getPreferences().syncPublicCityCatalog) {
            return SyncResult(pagesFetched = 0, complete = false, outcome = ServerStationsOutcome.Disabled)
        }
        var outcome = getServerStations(limit = PAGE_SIZE)
        var pages = if (outcome is ServerStationsOutcome.Fresh) 1 else 0
        while (outcome is ServerStationsOutcome.Fresh &&
            outcome.page.nextCursor != null &&
            pages < MAX_PAGES
        ) {
            if (!userPreferencesRepository.getPreferences().syncPublicCityCatalog) {
                return SyncResult(pagesFetched = pages, complete = false, outcome = outcome)
            }
            currentCoroutineContext().ensureActive()
            outcome = getServerStations.loadMore(limit = PAGE_SIZE)
            if (outcome is ServerStationsOutcome.Fresh) {
                pages++
            }
        }
        val complete = outcome is ServerStationsOutcome.Fresh && outcome.page.nextCursor == null
        return SyncResult(pagesFetched = pages, complete = complete, outcome = outcome)
    }

    companion object {
        const val PAGE_SIZE = 20

        /** At most 100 stations per run; the list offers more on demand. */
        const val MAX_PAGES = 5
    }
}
