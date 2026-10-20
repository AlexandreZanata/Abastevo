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
        enforceBudget(scopeKey)
    }

    @Transaction
    suspend fun appendPage(entities: List<ServerStationCacheEntity>, meta: ServerCatalogMetaEntity, scopeKey: String) {
        var position = nextPosition(scopeKey)
        for (entity in entities) {
            val existing = findScopedById(entity.stationId, scopeKey)
            upsert(entity.copy(scopeKey = scopeKey, position = existing?.position ?: position++))
        }
        upsertMeta(meta)
        enforceBudget(scopeKey)
    }

    @Transaction
    suspend fun removeScope(scopeKey: String) {
        clearScopePageRows(scopeKey)
        clearScopeMeta(scopeKey)
    }

    @Transaction
    suspend fun saveDetailBounded(entity: ServerStationCacheEntity) {
        upsert(entity)
        trimDetails()
        check(estimatedBytes() <= 32L * 1024 * 1024) { "directory cache storage budget exceeded" }
    }

    suspend fun enforceBudget(currentScope: String) {
        check(countScopePageRows(currentScope) <= 10_000) { "directory city station budget exceeded" }
        for (key in scopesOldestFirst().filter { it != currentScope }.take((scopeCount() - 8).coerceAtLeast(0))) removeScope(key)
        check(estimatedBytes() <= 32L * 1024 * 1024) { "directory cache storage budget exceeded" }
    }

    @Query("SELECT COALESCE(MAX(position), -1) + 1 FROM server_station_cache WHERE scope_key = :scopeKey AND position != 2147483647")
    suspend fun nextPosition(scopeKey: String): Int

    @Query("SELECT * FROM server_station_cache WHERE station_id = :stationId AND scope_key = :scopeKey LIMIT 1")
    suspend fun findScopedById(stationId: String, scopeKey: String): ServerStationCacheEntity?

    @Query("SELECT * FROM server_station_cache WHERE station_id = :stationId AND scope_key LIKE :namespace || '%' ORDER BY saved_at_millis DESC LIMIT 1")
    suspend fun findByIdInNamespace(stationId: String, namespace: String): ServerStationCacheEntity?

    @Query("SELECT `key` FROM server_catalog_meta ORDER BY saved_at_millis ASC, `key` ASC")
    suspend fun scopesOldestFirst(): List<String>

    @Query("SELECT COUNT(*) FROM server_catalog_meta")
    suspend fun scopeCount(): Int

    @Query("SELECT COALESCE(SUM(length(CAST(display_name AS BLOB)) + length(CAST(address_json AS BLOB)) + length(CAST(scope_key AS BLOB)) + 512), 0) FROM server_station_cache")
    suspend fun estimatedBytes(): Long

    @Query("DELETE FROM server_station_cache WHERE position = 2147483647 AND rowid NOT IN (SELECT rowid FROM server_station_cache WHERE position = 2147483647 ORDER BY saved_at_millis DESC, station_id ASC LIMIT 200)")
    suspend fun trimDetails()

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
    @Query("DELETE FROM server_station_cache WHERE position != 2147483647")
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
