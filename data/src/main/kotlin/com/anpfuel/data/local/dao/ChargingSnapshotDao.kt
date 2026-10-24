package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import androidx.room.Transaction
import com.anpfuel.data.local.charging.ChargingSnapshotImport
import com.anpfuel.data.local.entity.ChargingSnapshotMetaEntity
import com.anpfuel.data.local.entity.ChargingSnapshotPlaceEntity

/**
 * EE-18 public EV snapshot generations.
 *
 * A resnapshot writes the whole new generation, swaps the meta row and
 * deletes the old generation in one transaction: readers either see the
 * previous complete generation or the new one, never a mix. Interrupted
 * deltas leave IN_PROGRESS meta plus a resume cursor; the importer
 * revalidates the base before appending, so a stale delta can never
 * corrupt a newer generation. Disk budgets fail the transaction (Room
 * rolls back), never silently trim. Tombstoned places are invisible to
 * reads but keep their identity until the next resnapshot clears them.
 * Fuel, history and private tables are never touched here.
 */
@Dao
interface ChargingSnapshotDao {

    @Transaction
    suspend fun replaceGeneration(
        meta: ChargingSnapshotMetaEntity,
        places: List<ChargingSnapshotPlaceEntity>,
    ) {
        upsertPlaces(places)
        upsertMeta(meta)
        deleteOlderGenerations(meta.profile, meta.generation)
        enforceBudgets(meta.generation)
    }

    @Transaction
    suspend fun appendDelta(
        meta: ChargingSnapshotMetaEntity,
        places: List<ChargingSnapshotPlaceEntity>,
        tombstoneIds: List<String>,
    ) {
        upsertPlaces(places)
        if (tombstoneIds.isNotEmpty()) markTombstones(tombstoneIds, meta.generation)
        upsertMeta(meta)
        enforceBudgets(meta.generation)
    }

    @Transaction
    suspend fun removePack(profile: String) {
        deleteProfilePlaces(profile)
        clearMeta(ChargingSnapshotMetaEntity.packKey(profile))
    }

    suspend fun enforceBudgets(generation: Long) {
        check(countPlaces(generation) <= ChargingSnapshotImport.PLACES_BUDGET) { "snapshot places over budget" }
        check(estimatedBytes(generation) <= ChargingSnapshotImport.SNAPSHOT_BYTES_BUDGET) { "snapshot bytes over budget" }
    }

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertPlaces(places: List<ChargingSnapshotPlaceEntity>)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertMeta(meta: ChargingSnapshotMetaEntity)

    @Query("DELETE FROM charging_snapshot_place WHERE profile = :profile AND generation != :generation")
    suspend fun deleteOlderGenerations(profile: String, generation: Long)

    @Query("DELETE FROM charging_snapshot_place WHERE profile = :profile")
    suspend fun deleteProfilePlaces(profile: String)

    @Query("DELETE FROM charging_snapshot_meta WHERE `key` = :key")
    suspend fun clearMeta(key: String)

    @Query("UPDATE charging_snapshot_place SET tombstone = 1 WHERE place_id IN (:ids) AND generation = :generation")
    suspend fun markTombstones(ids: List<String>, generation: Long)

    @Query("SELECT * FROM charging_snapshot_meta WHERE `key` = :key LIMIT 1")
    suspend fun findMeta(key: String): ChargingSnapshotMetaEntity?

    @Query("SELECT * FROM charging_snapshot_place WHERE profile = :profile AND generation = :generation AND tombstone = 0 ORDER BY place_id LIMIT :limit OFFSET :offset")
    suspend fun readPlaces(profile: String, generation: Long, limit: Int, offset: Int): List<ChargingSnapshotPlaceEntity>

    @Query("SELECT * FROM charging_snapshot_place WHERE place_id = :placeId AND profile = :profile AND generation = :generation AND tombstone = 0 LIMIT 1")
    suspend fun findPlace(placeId: String, profile: String, generation: Long): ChargingSnapshotPlaceEntity?

    @Query("SELECT COUNT(*) FROM charging_snapshot_place WHERE generation = :generation AND tombstone = 0")
    suspend fun countPlaces(generation: Long): Long

    @Query("SELECT COALESCE(SUM(length(name) + length(groups_json) + 512), 0) FROM charging_snapshot_place WHERE generation = :generation")
    suspend fun estimatedBytes(generation: Long): Long
}
