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

    override suspend fun savePage(page: ServerStationPage) {
        dao.clearStations()
        dao.upsertAll(page.items.mapIndexed { index, station -> station.toEntity(index) })
        dao.upsertMeta(
            ServerCatalogMetaEntity(
                key = ServerCatalogMetaEntity.PAGE_KEY,
                nextCursor = page.nextCursor,
                savedAtMillis = System.currentTimeMillis(),
            ),
        )
    }

    override suspend fun loadPage(): ServerStationPage? {
        val entities = dao.loadPage()
        if (entities.isEmpty()) return null
        return ServerStationPage(
            items = entities.map { it.toDomain() },
            nextCursor = dao.findMeta(ServerCatalogMetaEntity.PAGE_KEY)?.nextCursor,
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
    )
}
