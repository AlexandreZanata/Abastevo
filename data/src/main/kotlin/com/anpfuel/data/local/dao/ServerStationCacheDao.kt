package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import androidx.room.Transaction
import com.anpfuel.data.local.entity.ServerCatalogMetaEntity
import com.anpfuel.data.local.entity.ServerStationCacheEntity

/**
 * P27-T03 canonical catalog DAO.
 *
 * Stations upsert by stable (UUID, scope) identity (replay converges);
 * each fresh traversal replaces **only its own scope**, so a city page
 * never wipes the national page, another municipality, or unscoped
 * detail records. Reads are scope-keyed with `LIMIT/OFFSET` windows
 * over the server `position` order; a stored meta row marks a valid
 * snapshot (an empty item set is a valid empty page, not a miss).
 * Details upsert singly so owner status and discussion targets resolve
 * offline. ANP tables are never referenced here.
 */
@Dao
interface ServerStationCacheDao {

    @Transaction
    suspend fun replacePage(
        entities: List<ServerStationCacheEntity>,
        meta: ServerCatalogMetaEntity,
        scopeKey: String = ServerCatalogMetaEntity.PAGE_KEY,
    ) {
        clearScopePageRows(scopeKey)
        upsertAll(entities.map { it.copy(scopeKey = scopeKey) })
        clearScopeMeta(scopeKey)
        upsertMeta(meta)
    }

    @Transaction
    suspend fun pageSnapshot(key: String = ServerCatalogMetaEntity.PAGE_KEY): Pair<List<ServerStationCacheEntity>, ServerCatalogMetaEntity?> =
        loadScopePage(key) to findMeta(key)

    @Transaction
    suspend fun pagedSnapshot(
        key: String = ServerCatalogMetaEntity.PAGE_KEY,
        limit: Int,
        offset: Int,
    ): Pair<List<ServerStationCacheEntity>, ServerCatalogMetaEntity?> =
        loadScopePageWindow(key, limit, offset) to findMeta(key)

    @Query("SELECT * FROM server_station_cache WHERE scope_key = :scopeKey AND position != 2147483647 ORDER BY position ASC")
    suspend fun loadScopePage(scopeKey: String): List<ServerStationCacheEntity>

    @Query("SELECT * FROM server_station_cache WHERE scope_key = :scopeKey AND position != 2147483647 ORDER BY position ASC LIMIT :limit OFFSET :offset")
    suspend fun loadScopePageWindow(scopeKey: String, limit: Int, offset: Int): List<ServerStationCacheEntity>

    @Query("SELECT COUNT(*) FROM server_station_cache WHERE scope_key = :scopeKey AND position != 2147483647")
    suspend fun countScopePageRows(scopeKey: String): Int

    @Query("SELECT * FROM server_station_cache ORDER BY position ASC")
    suspend fun loadPage(): List<ServerStationCacheEntity>

    @Query("SELECT * FROM server_station_cache WHERE station_id = :stationId LIMIT 1")
    suspend fun findById(stationId: String): ServerStationCacheEntity?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(entities: List<ServerStationCacheEntity>)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(entity: ServerStationCacheEntity)

    @Transaction
    suspend fun clearPublicCatalog() {
        clearPublicPageRows()
        clearMeta()
    }

    @Query("DELETE FROM server_station_cache WHERE scope_key = :scopeKey AND position != 2147483647")
    suspend fun clearScopePageRows(scopeKey: String)

    /** Drops every page row in every scope; unscoped detail records survive. */
    @Query("DELETE FROM server_station_cache WHERE scope_key != ''")
    suspend fun clearPublicPageRows()

    @Query("DELETE FROM server_station_cache WHERE position != 2147483647")
    suspend fun clearPageRows()

    @Query("DELETE FROM server_station_cache")
    suspend fun clearStations()

    @Query("SELECT * FROM server_catalog_meta WHERE `key` = :key LIMIT 1")
    suspend fun findMeta(key: String): ServerCatalogMetaEntity?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertMeta(meta: ServerCatalogMetaEntity)

    @Query("DELETE FROM server_catalog_meta WHERE `key` = :scopeKey")
    suspend fun clearScopeMeta(scopeKey: String)

    @Query("DELETE FROM server_catalog_meta")
    suspend fun clearMeta()
}
