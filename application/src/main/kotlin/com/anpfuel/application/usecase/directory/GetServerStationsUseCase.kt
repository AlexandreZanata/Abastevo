package com.anpfuel.application.usecase.directory

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import com.anpfuel.application.port.CommunityReadsFlagProvider
import com.anpfuel.application.port.StationDirectoryScopeProvider
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.ServerStationPage
import com.anpfuel.domain.repository.ServerStationCache
import com.anpfuel.domain.repository.ServerStationGateway

/**
 * P35-T02 server-backed discovery reads.
 *
 * Community-first station browsing against canonical Directory UUIDs;
 * the dated ANP reference stays a separate secondary read (existing
 * price-group ports). Gated by the community-reads flag (OFF by
 * default, rollback OFF): disabled never touches network or cache and
 * legacy ANP/offline journeys keep working. Network success saves the
 * last good page/detail; transport failure replays last-known cache
 * honestly (`StaleCache`) or reports `Unavailable` — never an invented
 * station and never a silent rewrite of saved legacy rows.
 */
sealed interface ServerStationsOutcome {
    data object Disabled : ServerStationsOutcome
    data class Fresh(val page: ServerStationPage) : ServerStationsOutcome
    data class StaleCache(val page: ServerStationPage, val cause: Exception) : ServerStationsOutcome
    data class Unavailable(val cause: Exception) : ServerStationsOutcome
}

sealed interface ServerStationDetailOutcome {
    data object Disabled : ServerStationDetailOutcome
    data class Fresh(val station: ServerStation) : ServerStationDetailOutcome
    data class StaleCache(val station: ServerStation, val cause: Exception) : ServerStationDetailOutcome
    data class Unavailable(val cause: Exception) : ServerStationDetailOutcome
}

class GetServerStationsUseCase(
    private val flagProvider: CommunityReadsFlagProvider,
    private val gateway: ServerStationGateway,
    private val cache: ServerStationCache,
    private val scopeProvider: StationDirectoryScopeProvider? = null,
) {
    private val refreshLock = Mutex()

    suspend fun cached(): ServerStationsOutcome {
        if (!flagProvider.isEnabled()) return ServerStationsOutcome.Disabled
        val cause = java.io.IOException("directory offline")
        return try {
            val scope = scopeProvider?.municipalityCode()
            if (scopeProvider != null && scope == null) return ServerStationsOutcome.Unavailable(cause)
            loadCache(scope)?.let { ServerStationsOutcome.StaleCache(it, cause) }
                ?: ServerStationsOutcome.Unavailable(cause)
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            ServerStationsOutcome.Unavailable(error)
        }
    }

    suspend operator fun invoke(limit: Int = 20, cursor: String? = null): ServerStationsOutcome = refreshLock.withLock {
        if (!flagProvider.isEnabled()) return@withLock ServerStationsOutcome.Disabled
        return@withLock when (val resolution = resolveScope()) {
            is ScopeResolution.Missing -> ServerStationsOutcome.Unavailable(
                java.io.IOException("directory city unavailable"),
            )
            is ScopeResolution.Failed -> ServerStationsOutcome.Unavailable(resolution.error)
            is ScopeResolution.Present -> fresh(limit, cursor, resolution.scope)
        }
    }

    /**
     * Appends the next page to the displayed list. One continuation at a
     * time (shared [refreshLock]); repeats are deduplicated by stable
     * UUID; the accumulated list is cached so offline replay and process
     * death keep every loaded page. An expired/invalid cursor (HTTP 400)
     * restarts from the first page instead of stranding the list; any
     * other failure replays the last good snapshot so displayed data is
     * never wiped by a retry.
     */
    suspend fun loadMore(limit: Int = 20): ServerStationsOutcome = refreshLock.withLock {
        if (!flagProvider.isEnabled()) return@withLock ServerStationsOutcome.Disabled
        val scope = when (val resolution = resolveScope()) {
            is ScopeResolution.Missing -> return@withLock ServerStationsOutcome.Unavailable(
                java.io.IOException("directory city unavailable"),
            )
            is ScopeResolution.Failed -> return@withLock ServerStationsOutcome.Unavailable(resolution.error)
            is ScopeResolution.Present -> resolution.scope
        }
        val cached = loadCache(scope) ?: return@withLock fresh(limit, null, scope)
        val cursor = cached.nextCursor ?: return@withLock ServerStationsOutcome.Fresh(cached)
        try {
            val page = if (scope == null) gateway.list(limit, cursor)
                else gateway.listInMunicipality(scope, limit, cursor)
            require(scope == null || page.items.all { it.municipalityCode == scope }) { "directory city mismatch" }
            currentCoroutineContext().ensureActive()
            val merged = ServerStationPage(
                items = (cached.items + page.items).distinctBy { it.stationId },
                nextCursor = page.nextCursor,
            )
            if (scope == null) cache.savePage(merged) else cache.saveScopedPage(scope, merged)
            ServerStationsOutcome.Fresh(merged)
        } catch (error: CancellationException) {
            throw error
        } catch (error: com.anpfuel.domain.discovery.DirectoryCursorExpiredException) {
            fresh(limit, null, scope)
        } catch (error: Exception) {
            ServerStationsOutcome.StaleCache(cached, error)
        }
    }

    private sealed interface ScopeResolution {
        data object Missing : ScopeResolution
        data class Failed(val error: Exception) : ScopeResolution
        data class Present(val scope: String?) : ScopeResolution
    }

    /** City scope, distinguishing a missing required binding from a national read. */
    private suspend fun resolveScope(): ScopeResolution {
        return try {
            val scope = scopeProvider?.municipalityCode()
            if (scopeProvider != null && scope == null) ScopeResolution.Missing
            else ScopeResolution.Present(scope)
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            ScopeResolution.Failed(error)
        }
    }

    /** First-page fetch with last-good fallback. Callers hold [refreshLock]. */
    private suspend fun fresh(limit: Int, cursor: String?, scope: String?): ServerStationsOutcome {
        return try {
            val page = if (scope == null) gateway.list(limit, cursor)
                else gateway.listInMunicipality(scope, limit, cursor)
            require(scope == null || page.items.all { it.municipalityCode == scope }) { "directory city mismatch" }
            currentCoroutineContext().ensureActive()
            if (scope == null) cache.savePage(page) else cache.saveScopedPage(scope, page)
            ServerStationsOutcome.Fresh(page)
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            val cached = runCatching { loadCache(scope) }.onFailure { if (it is CancellationException) throw it }.getOrNull()
            if (cached != null) ServerStationsOutcome.StaleCache(cached, error)
            else ServerStationsOutcome.Unavailable(error)
        }
    }

    private suspend fun loadCache(scope: String?): ServerStationPage? =
        if (scope == null) cache.loadPage() else cache.loadScopedPage(scope)
}

class GetServerStationDetailUseCase(
    private val flagProvider: CommunityReadsFlagProvider,
    private val gateway: ServerStationGateway,
    private val cache: ServerStationCache,
) {
    suspend operator fun invoke(stationId: String): ServerStationDetailOutcome {
        if (!flagProvider.isEnabled()) return ServerStationDetailOutcome.Disabled
        if (stationId.isBlank()) throw IllegalArgumentException("station_id is blank")
        return try {
            val station = gateway.detail(stationId)
            currentCoroutineContext().ensureActive()
            cache.saveDetail(station)
            ServerStationDetailOutcome.Fresh(station)
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            val cached = runCatching { cache.loadDetail(stationId) }.onFailure { if (it is CancellationException) throw it }.getOrNull()
            if (cached != null) ServerStationDetailOutcome.StaleCache(cached, error)
            else ServerStationDetailOutcome.Unavailable(error)
        }
    }
}

/**
 * P35-T03 bounded nearby lookup.
 *
 * Explicit-permission transient GPS only: coordinates are passed in by the
 * caller (which owns the permission flow), never persisted, logged or
 * cached here. Bounds (lat/lon/radius/limit) are refused before any
 * network call. No last-known replay: a failed nearby lookup reports
 * `Unavailable` honestly instead of a stale position.
 */
sealed interface NearbyServerStationsOutcome {
    data object Disabled : NearbyServerStationsOutcome
    data class Fresh(val stations: List<com.anpfuel.domain.discovery.NearbyServerStation>) : NearbyServerStationsOutcome
    data class Unavailable(val cause: Exception) : NearbyServerStationsOutcome
}

class GetNearbyServerStationsUseCase(
    private val flagProvider: CommunityReadsFlagProvider,
    private val gateway: ServerStationGateway,
) {
    suspend operator fun invoke(
        lat: Double,
        lon: Double,
        radiusMeters: Int = 2000,
        limit: Int = 20,
    ): NearbyServerStationsOutcome {
        if (!flagProvider.isEnabled()) return NearbyServerStationsOutcome.Disabled
        return try {
            NearbyServerStationsOutcome.Fresh(gateway.nearby(lat, lon, radiusMeters, limit))
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            NearbyServerStationsOutcome.Unavailable(error)
        }
    }
}
