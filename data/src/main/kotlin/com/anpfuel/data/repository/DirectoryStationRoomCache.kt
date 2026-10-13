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
 * failed-refresh recovery. Fresh traversals replace the page set so
 * stale scopes never linger; details upsert singly. Nearby stays
 * transient (never persisted, B-BR-011).
 */
@Singleton
class DirectoryStationRoomCache @Inject constructor(
    private val dao: ServerStationCacheDao,
) : ServerStationCache {

    override suspend fun savePage(page: ServerStationPage) = savePage(page, ServerCatalogMetaEntity.PAGE_KEY)

    override suspend fun saveScopedPage(municipalityCode: String, page: ServerStationPage) =
        savePage(page, "city:$municipalityCode")

    private suspend fun savePage(page: ServerStationPage, key: String) {
        dao.replacePage(
            page.items.mapIndexed { index, station -> station.toEntity(index) },
            ServerCatalogMetaEntity(
                key = key,
                nextCursor = page.nextCursor,
                savedAtMillis = System.currentTimeMillis(),
            ),
        )
    }

    override suspend fun loadPage(): ServerStationPage? = loadPage(ServerCatalogMetaEntity.PAGE_KEY)

    override suspend fun loadScopedPage(municipalityCode: String): ServerStationPage? =
        loadPage("city:$municipalityCode")?.takeIf { page -> page.items.all { it.municipalityCode == municipalityCode } }

    private suspend fun loadPage(key: String): ServerStationPage? {
        val (entities, meta) = dao.pageSnapshot(key)
        if (meta == null) return null
        return ServerStationPage(
            items = entities.filter { it.position != Int.MAX_VALUE }.map { it.toDomain() },
            nextCursor = meta.nextCursor,
        )
    }

    override suspend fun saveDetail(station: ServerStation) {
        val existing = dao.findById(station.stationId)
        dao.upsert(station.toEntity(existing?.position ?: Int.MAX_VALUE))
    }

    override suspend fun loadDetail(stationId: String): ServerStation? =
        dao.findById(stationId.lowercase())?.toDomain()

    override suspend fun clear() {
        dao.clearStations()
        dao.clearMeta()
    }

    private fun ServerStation.toEntity(position: Int) = ServerStationCacheEntity(
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
