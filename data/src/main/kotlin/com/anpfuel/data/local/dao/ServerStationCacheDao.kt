package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import com.anpfuel.data.local.entity.ServerCatalogMetaEntity
import com.anpfuel.data.local.entity.ServerStationCacheEntity

/**
 * P27-T03 canonical catalog DAO.
 *
 * Stations upsert by stable UUID (replay converges); the page set is
 * replaced per fresh traversal (stale rows from a previous scope never
 * linger beside new ones). Details upsert singly so owner status and
 * discussion targets resolve offline.
 */
@Dao
interface ServerStationCacheDao {

    @Query("SELECT * FROM server_station_cache ORDER BY position ASC")
    suspend fun loadPage(): List<ServerStationCacheEntity>

    @Query("SELECT * FROM server_station_cache WHERE station_id = :stationId LIMIT 1")
    suspend fun findById(stationId: String): ServerStationCacheEntity?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(entities: List<ServerStationCacheEntity>)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(entity: ServerStationCacheEntity)

    @Query("DELETE FROM server_station_cache")
    suspend fun clearStations()

    @Query("SELECT * FROM server_catalog_meta WHERE `key` = :key LIMIT 1")
    suspend fun findMeta(key: String): ServerCatalogMetaEntity?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertMeta(meta: ServerCatalogMetaEntity)

    @Query("DELETE FROM server_catalog_meta")
    suspend fun clearMeta()
}
