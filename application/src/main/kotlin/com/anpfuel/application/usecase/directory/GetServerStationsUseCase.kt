package com.anpfuel.application.usecase.directory

import com.anpfuel.application.port.CommunityReadsFlagProvider
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
) {
    suspend operator fun invoke(limit: Int = 20, cursor: String? = null): ServerStationsOutcome {
        if (!flagProvider.isEnabled()) return ServerStationsOutcome.Disabled
        return try {
            val page = gateway.list(limit, cursor)
            cache.savePage(page)
            ServerStationsOutcome.Fresh(page)
        } catch (error: Exception) {
            val cached = runCatching { cache.loadPage() }.getOrNull()
            if (cached != null) ServerStationsOutcome.StaleCache(cached, error)
            else ServerStationsOutcome.Unavailable(error)
        }
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
            cache.saveDetail(station)
            ServerStationDetailOutcome.Fresh(station)
        } catch (error: Exception) {
            val cached = runCatching { cache.loadDetail(stationId) }.getOrNull()
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
        } catch (error: Exception) {
            NearbyServerStationsOutcome.Unavailable(error)
        }
    }
}
