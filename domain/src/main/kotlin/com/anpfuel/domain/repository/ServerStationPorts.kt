package com.anpfuel.domain.repository

import com.anpfuel.domain.discovery.NearbyServerStation
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.ServerStationPage

/**
 * P35-T01 canonical Directory read ports.
 *
 * HTTP is network-only (throws on transport/parse failure, never returns
 * cache); the memory cache keeps the last good page/detail for
 * failed-refresh recovery. Legacy CNPJ/offline rows are never rewritten
 * here; callers resolve old full-CNPJ identities explicitly. No Room
 * migration is introduced by this slice.
 */
interface ServerStationGateway {
    suspend fun list(limit: Int, cursor: String?): ServerStationPage

    suspend fun nearby(
        lat: Double,
        lon: Double,
        radiusMeters: Int,
        limit: Int,
    ): List<NearbyServerStation>

    suspend fun detail(stationId: String): ServerStation
}

interface ServerStationCache {
    fun savePage(page: ServerStationPage)

    fun loadPage(): ServerStationPage?

    fun saveDetail(station: ServerStation)

    fun loadDetail(stationId: String): ServerStation?

    fun clear()
}
