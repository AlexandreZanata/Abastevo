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

    suspend fun search(
        municipalityCode: String,
        query: String,
        limit: Int = 20,
        cursor: String? = null,
    ): ServerStationPage = throw UnsupportedOperationException("station search unavailable")

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

    /** Append one bounded remote page; concrete persistence implementations do this transactionally. */
    suspend fun appendPage(page: ServerStationPage) {
        val prior = loadPage()
        savePage(ServerStationPage((prior?.items.orEmpty() + page.items).distinctBy { it.stationId }, page.nextCursor))
    }

    suspend fun appendScopedPage(scope: String, page: ServerStationPage) {
        val prior = loadScopedPage(scope)
        saveScopedPage(scope, ServerStationPage((prior?.items.orEmpty() + page.items).distinctBy { it.stationId }, page.nextCursor))
    }

    suspend fun removeScopedPage(scope: String) { saveScopedPage(scope, ServerStationPage(emptyList(), null)) }

    /** Windowed read over the stored server order for virtualized lists. */
    suspend fun loadPage(limit: Int, offset: Int): ServerStationPage? = loadPage()?.let {
        it.copy(items = it.items.drop(offset).take(limit), cachedNextOffset = if (offset + limit < it.items.size) offset + limit else null)
    }

    /** Windowed read over the stored city order for virtualized lists. */
    suspend fun loadScopedPage(municipalityCode: String, limit: Int, offset: Int): ServerStationPage? = loadScopedPage(municipalityCode)?.let {
        it.copy(items = it.items.drop(offset).take(limit), cachedNextOffset = if (offset + limit < it.items.size) offset + limit else null)
    }

    /** Drops stored directory pages (every scope) while keeping details. */
    suspend fun clearPublicPages(): Unit =
        throw UnsupportedOperationException("public catalog clearing unavailable")

    suspend fun saveDetail(station: ServerStation)

    suspend fun loadDetail(stationId: String): ServerStation?

    suspend fun clear()
}
