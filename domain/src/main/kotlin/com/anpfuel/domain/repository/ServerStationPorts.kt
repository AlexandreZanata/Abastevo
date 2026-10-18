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
 * here; callers resolve old full-CNPJ identities explicitly. Page rows
 * are scope-keyed (national page vs `city:<ibge>`); see the Room cache
 * for the scoped persistence contract.
 */
interface ServerStationGateway {
    suspend fun list(limit: Int, cursor: String?): ServerStationPage
    suspend fun listInMunicipality(municipalityCode: String, limit: Int, cursor: String?): ServerStationPage {
        require(cursor == null) { "city pagination unavailable" }
        return search(municipalityCode, "", limit)
    }

    suspend fun search(municipalityCode: String, query: String, limit: Int = 20): ServerStationPage =
        throw UnsupportedOperationException("station search unavailable")

    suspend fun nearby(
        lat: Double,
        lon: Double,
        radiusMeters: Int,
        limit: Int,
    ): List<NearbyServerStation>

    suspend fun detail(stationId: String): ServerStation

    /** Existing active identifier only; never creates a station. */
    suspend fun byCnpj(cnpj: String): ServerStation? = throw UnsupportedOperationException("identifier lookup unavailable")
}

interface ServerStationCache {
    suspend fun saveScopedPage(municipalityCode: String, page: ServerStationPage): Unit =
        throw UnsupportedOperationException("scoped directory cache unavailable")

    suspend fun loadScopedPage(municipalityCode: String): ServerStationPage? =
        throw UnsupportedOperationException("scoped directory cache unavailable")

    suspend fun savePage(page: ServerStationPage)

    suspend fun loadPage(): ServerStationPage?

    /** Windowed read over the stored server order for virtualized lists. */
    suspend fun loadPage(limit: Int, offset: Int): ServerStationPage? =
        throw UnsupportedOperationException("paged directory cache unavailable")

    /** Windowed read over the stored city order for virtualized lists. */
    suspend fun loadScopedPage(municipalityCode: String, limit: Int, offset: Int): ServerStationPage? =
        throw UnsupportedOperationException("paged directory cache unavailable")

    suspend fun saveDetail(station: ServerStation)

    suspend fun loadDetail(stationId: String): ServerStation?

    suspend fun clear()
}
