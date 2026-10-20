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
    data class Fresh(val page: ServerStationPage, val fromCache: Boolean = false) : ServerStationsOutcome
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

    private suspend fun scope(): String? {
        val city = scopeProvider?.municipalityCode()
        check(scopeProvider == null || city != null) { "directory city unavailable" }
        return city
    }

    private fun partition(city: String?, query: String): String? =
        if (query.isEmpty()) city else "${city ?: "national"}|query:$query"

    private suspend fun load(key: String?): ServerStationPage? =
        if (key == null) cache.loadPage() else cache.loadScopedPage(key)

    private suspend fun save(key: String?, page: ServerStationPage) {
        if (key == null) cache.savePage(page) else cache.saveScopedPage(key, page)
    }

    private suspend fun offlineWindow(city: String?, query: String, limit: Int, offset: Int): ServerStationPage? {
        if (query.isEmpty() && scopeProvider != null) {
            val downloaded = cache.loadScopedPage("${city ?: "national"}|download", limit, offset)
            if (downloaded != null) return downloaded
        }
        val key = partition(city, query)
        return if (key == null) cache.loadPage(limit, offset) else cache.loadScopedPage(key, limit, offset)
    }

    suspend fun cached(query: String = ""): ServerStationsOutcome {
        if (!flagProvider.isEnabled()) return ServerStationsOutcome.Disabled
        return try {
            val page = offlineWindow(scope(), query.trim(), 20, 0)
            val error = java.io.IOException("directory offline")
            if (page == null) ServerStationsOutcome.Unavailable(error) else ServerStationsOutcome.StaleCache(page, error)
        } catch (error: CancellationException) { throw error
        } catch (error: Exception) { ServerStationsOutcome.Unavailable(error) }
    }

    suspend operator fun invoke(limit: Int = 20, cursor: String? = null, query: String = ""): ServerStationsOutcome = refreshLock.withLock {
        if (!flagProvider.isEnabled()) return@withLock ServerStationsOutcome.Disabled
        try {
            val city = scope()
            fresh(limit, cursor, city, query.trim(), partition(city, query.trim()))
        } catch (error: CancellationException) { throw error
        } catch (error: Exception) { ServerStationsOutcome.Unavailable(error) }
    }

    suspend fun loadMore(limit: Int = 20, query: String = "", cachedOffset: Int? = null): ServerStationsOutcome {
        if (!flagProvider.isEnabled()) return ServerStationsOutcome.Disabled
        if (cachedOffset != null) {
            return try {
                val window = offlineWindow(scope(), query.trim(), limit, cachedOffset)
                    ?: return ServerStationsOutcome.Unavailable(java.io.IOException("directory cached page unavailable"))
                ServerStationsOutcome.Fresh(window, fromCache = true)
            } catch (error: CancellationException) { throw error
            } catch (error: Exception) { ServerStationsOutcome.Unavailable(error) }
        }
        return refreshLock.withLock {
            try {
                val city = scope()
                more(limit, city, query.trim(), partition(city, query.trim()))
            } catch (error: CancellationException) { throw error
            } catch (error: Exception) { ServerStationsOutcome.Unavailable(error) }
        }
    }

    /** Clear serializes with every public read and sync batch: a retired write cannot repopulate it. */
    suspend fun clearPublicPages() = refreshLock.withLock { cache.clearPublicPages() }

    /** A separate persisted staging scope resumes bounded batches without replacing the usable city. */
    suspend fun syncBatch(limit: Int, maxPages: Int, allowed: suspend () -> Boolean): SyncPublicCatalogUseCase.SyncResult = refreshLock.withLock {
        var fetched = 0
        var last: ServerStationsOutcome = ServerStationsOutcome.Disabled
        if (!flagProvider.isEnabled() || !allowed()) return@withLock SyncPublicCatalogUseCase.SyncResult(0, false, last)
        try {
            val city = scope()
            val stage = "${city ?: "national"}|background"
            var saved = load(stage)
            while (fetched < maxPages && allowed()) {
                currentCoroutineContext().ensureActive()
                last = if (saved?.nextCursor != null) more(limit, city, "", stage, allowed)
                    else fresh(limit, null, city, "", stage, allowed)
                if (last !is ServerStationsOutcome.Fresh) break
                fetched++
                saved = last.page
                if (saved.nextCursor == null) {
                    if (!allowed() || scope() != city) return@withLock SyncPublicCatalogUseCase.SyncResult(fetched, false, ServerStationsOutcome.Disabled)
                    cache.saveScopedPage("${city ?: "national"}|download", saved)
                    cache.removeScopedPage(stage)
                    return@withLock SyncPublicCatalogUseCase.SyncResult(fetched, true, last)
                }
            }
            SyncPublicCatalogUseCase.SyncResult(fetched, false, last)
        } catch (error: CancellationException) { throw error
        } catch (error: Exception) { SyncPublicCatalogUseCase.SyncResult(fetched, false, ServerStationsOutcome.Unavailable(error)) }
    }

    private suspend fun request(limit: Int, cursor: String?, city: String?, query: String): ServerStationPage {
        require(limit in 1..100)
        require(query.isEmpty() || query.length in 2..100)
        val page = when {
            query.isNotEmpty() -> gateway.search(requireNotNull(city) { "search city unavailable" }, query, limit, cursor)
            city == null -> gateway.list(limit, cursor)
            else -> gateway.listInMunicipality(city, limit, cursor)
        }
        require(page.items.size <= limit) { "directory page exceeds requested limit" }
        require(city == null || page.items.all { it.municipalityCode == city }) { "directory city mismatch" }
        return page
    }

    private suspend fun canCommit(city: String?, allowed: suspend () -> Boolean): Boolean {
        currentCoroutineContext().ensureActive()
        return flagProvider.isEnabled() && allowed() && scope() == city
    }

    private suspend fun fresh(limit: Int, cursor: String?, city: String?, query: String, key: String?, allowed: suspend () -> Boolean = { true }): ServerStationsOutcome {
        return try {
            val page = request(limit, cursor, city, query)
            if (!canCommit(city, allowed)) return ServerStationsOutcome.Disabled
            save(key, page)
            ServerStationsOutcome.Fresh(page)
        } catch (error: CancellationException) { throw error
        } catch (error: Exception) {
            val cached = try {
                if (key == partition(city, query)) offlineWindow(city, query, limit, 0) else load(key)
            } catch (cancelled: CancellationException) { throw cancelled } catch (_: Exception) { null }
            if (cached != null) ServerStationsOutcome.StaleCache(cached, error) else ServerStationsOutcome.Unavailable(error)
        }
    }

    private suspend fun more(limit: Int, city: String?, query: String, key: String?, allowed: suspend () -> Boolean = { true }): ServerStationsOutcome {
        val prior = load(key) ?: return fresh(limit, null, city, query, key, allowed)
        val cursor = prior.nextCursor ?: return ServerStationsOutcome.Fresh(prior)
        return try {
            val page = request(limit, cursor, city, query)
            require(page.nextCursor != cursor) { "directory cursor did not advance" }
            val ids = prior.items.map { it.stationId }.toHashSet()
            require(page.nextCursor == null || page.items.any { it.stationId !in ids }) { "directory page did not advance" }
            if (!canCommit(city, allowed)) return ServerStationsOutcome.Disabled
            val merged = ServerStationPage((prior.items + page.items).distinctBy { it.stationId }, page.nextCursor)
            if (key == null) cache.appendPage(page) else cache.appendScopedPage(key, page)
            ServerStationsOutcome.Fresh(merged)
        } catch (error: CancellationException) { throw error
        } catch (error: com.anpfuel.domain.discovery.DirectoryCursorExpiredException) { fresh(limit, null, city, query, key, allowed)
        } catch (error: Exception) { ServerStationsOutcome.StaleCache(prior, error) }
    }
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
