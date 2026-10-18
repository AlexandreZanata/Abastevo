package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.ServerStationCacheDao
import com.anpfuel.data.local.entity.ServerCatalogMetaEntity
import com.anpfuel.data.local.entity.ServerStationCacheEntity
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.ServerStationPage
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.repository.ServerStationCache
import javax.inject.Inject
import javax.inject.Singleton

/**
 * P27-T03 Room-backed canonical catalog cache.
 *
 * Survives process death (offline compatibility): the last good page
 * and details replay without network, exactly like the legacy
 * failed-refresh recovery. Each scope (national page,
 * `city:<ibge>`) owns its rows and cursor: saving one scope never
 * deletes another scope, unscoped details, or ANP tables, and a
 * missing meta row means "never fetched" while a stored meta with
 * zero rows is an honest empty page. Paged reads window the server
 * `position` order for virtualized lists. Nearby stays transient
 * (never persisted, B-BR-011).
 */
@Singleton
class DirectoryStationRoomCache @Inject constructor(
    private val dao: ServerStationCacheDao,
) : ServerStationCache {

    override suspend fun savePage(page: ServerStationPage) = savePage(page, ServerCatalogMetaEntity.PAGE_KEY)

    override suspend fun saveScopedPage(municipalityCode: String, page: ServerStationPage) =
        savePage(page, scopeKey(municipalityCode))

    private suspend fun savePage(page: ServerStationPage, key: String) {
        dao.replacePage(
            page.items.mapIndexed { index, station -> station.toEntity(index, key) },
            ServerCatalogMetaEntity(
                key = key,
                nextCursor = page.nextCursor,
                savedAtMillis = System.currentTimeMillis(),
            ),
            key,
        )
    }

    override suspend fun loadPage(): ServerStationPage? = loadPage(ServerCatalogMetaEntity.PAGE_KEY)

    override suspend fun loadScopedPage(municipalityCode: String): ServerStationPage? =
        loadPage(scopeKey(municipalityCode))

    override suspend fun loadPage(limit: Int, offset: Int): ServerStationPage? =
        loadPage(ServerCatalogMetaEntity.PAGE_KEY, limit, offset)

    override suspend fun loadScopedPage(municipalityCode: String, limit: Int, offset: Int): ServerStationPage? =
        loadPage(scopeKey(municipalityCode), limit, offset)

    private suspend fun loadPage(key: String): ServerStationPage? {
        val (entities, meta) = dao.pageSnapshot(key)
        return snapshot(entities, meta)
    }

    private suspend fun loadPage(key: String, limit: Int, offset: Int): ServerStationPage? {
        require(limit in 1..MAX_PAGE_READ && offset >= 0) { "page window out of bounds" }
        val (entities, meta) = dao.pagedSnapshot(key, limit, offset)
        return snapshot(entities, meta)
    }

    private fun snapshot(
        entities: List<ServerStationCacheEntity>,
        meta: ServerCatalogMetaEntity?,
    ): ServerStationPage? {
        if (meta == null) return null
        return ServerStationPage(
            items = entities.filter { it.position != Int.MAX_VALUE }.map { it.toDomain() },
            nextCursor = meta.nextCursor,
        )
    }

    override suspend fun saveDetail(station: ServerStation) {
        val existing = dao.findById(station.stationId)
        dao.upsert(station.toEntity(existing?.position ?: Int.MAX_VALUE, ServerStationCacheEntity.DETAIL_SCOPE))
    }

    override suspend fun loadDetail(stationId: String): ServerStation? =
        dao.findById(stationId.lowercase())?.toDomain()

    override suspend fun clearPublicPages() {
        dao.clearPublicCatalog()
    }

    override suspend fun clear() {
        dao.clearStations()
        dao.clearMeta()
    }

    private fun scopeKey(municipalityCode: String) = "city:$municipalityCode"

    companion object {
        /** Largest window a virtualized list may read; full snapshots bypass it internally. */
        const val MAX_PAGE_READ = 100
    }

    private fun ServerStation.toEntity(position: Int, scopeKey: String) = ServerStationCacheEntity(
        stationId = stationId,
        displayName = displayName,
        locationQuality = locationQuality.wire,
        latitude = latitude,
        longitude = longitude,
        cnpjNormalized = cnpjNormalized,
        municipalityCode = municipalityCode,
        state = state,
        currentRevisionId = currentRevisionId,
        position = position,
        savedAtMillis = System.currentTimeMillis(),
        addressJson = org.json.JSONObject(address).toString(),
        scopeKey = scopeKey,
    )

    private fun ServerStationCacheEntity.toDomain() = ServerStation.create(
        stationId = stationId,
        displayName = displayName,
        locationQuality = StationLocationQuality.fromWire(locationQuality),
        latitude = latitude,
        longitude = longitude,
        cnpjNormalized = cnpjNormalized,
        municipalityCode = municipalityCode,
        state = state,
        currentRevisionId = currentRevisionId,
        address = org.json.JSONObject(addressJson).let { doc -> doc.keys().asSequence().associateWith { doc.getString(it) } },
    )
}
