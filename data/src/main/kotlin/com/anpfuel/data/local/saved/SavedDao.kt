package com.anpfuel.data.local.saved

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query

@Dao
interface SavedDao {
    @Query("SELECT * FROM map_saved_place WHERE owner_account = :owner AND origin = :origin ORDER BY created_at_millis DESC, place_id")
    suspend fun places(owner: String, origin: String): List<SavedPlaceEntity>

    @Query("SELECT * FROM map_saved_place WHERE owner_account = :owner AND origin = :origin AND station_id = :stationId")
    suspend fun placeByStation(owner: String, origin: String, stationId: String): SavedPlaceEntity?

    @Query("SELECT * FROM map_saved_place WHERE owner_account = :owner AND origin = :origin AND place_id = :id")
    suspend fun place(owner: String, origin: String, id: String): SavedPlaceEntity?

    @Query("SELECT COUNT(*) FROM map_saved_place WHERE owner_account = :owner AND origin = :origin")
    suspend fun placeCount(owner: String, origin: String): Int

    @Insert(onConflict = OnConflictStrategy.ABORT)
    suspend fun insertPlace(place: SavedPlaceEntity)

    @Query("DELETE FROM map_saved_place WHERE owner_account = :owner AND origin = :origin AND place_id = :id")
    suspend fun deletePlace(owner: String, origin: String, id: String): Int

    @Query("SELECT * FROM map_saved_trip WHERE owner_account = :owner AND origin = :origin AND state = 'COMPLETE' ORDER BY created_at_millis DESC, trip_id")
    suspend fun trips(owner: String, origin: String): List<SavedTripEntity>

    @Query("SELECT * FROM map_saved_trip WHERE owner_account = :owner AND origin = :origin AND trip_id = :id")
    suspend fun tripAnyState(owner: String, origin: String, id: String): SavedTripEntity?

    @Query("SELECT * FROM map_saved_trip WHERE owner_account = :owner AND origin = :origin AND state = 'RECORDING' LIMIT 1")
    suspend fun recording(owner: String, origin: String): SavedTripEntity?

    @Query("SELECT COUNT(*) FROM map_saved_trip WHERE owner_account = :owner AND origin = :origin AND state = 'COMPLETE'")
    suspend fun tripCount(owner: String, origin: String): Int

    @Insert(onConflict = OnConflictStrategy.ABORT)
    suspend fun insertTrip(trip: SavedTripEntity)

    @Insert(onConflict = OnConflictStrategy.ABORT)
    suspend fun insertPoints(points: List<SavedTripPointEntity>)

    @Query("SELECT * FROM map_saved_trip_point WHERE owner_account = :owner AND origin = :origin AND trip_id = :tripId ORDER BY seq")
    suspend fun points(owner: String, origin: String, tripId: String): List<SavedTripPointEntity>

    @Query("UPDATE map_saved_trip SET point_count = :count WHERE owner_account = :owner AND origin = :origin AND trip_id = :id")
    suspend fun setPointCount(owner: String, origin: String, id: String, count: Int)

    @Query("UPDATE map_saved_trip SET state = 'COMPLETE', point_count = :count WHERE owner_account = :owner AND origin = :origin AND trip_id = :id AND state = 'RECORDING'")
    suspend fun completeRecording(owner: String, origin: String, id: String, count: Int): Int

    @Query("DELETE FROM map_saved_trip WHERE owner_account = :owner AND origin = :origin AND trip_id = :id")
    suspend fun deleteTrip(owner: String, origin: String, id: String): Int

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun tombstone(row: SavedTombstoneEntity)

    @Query("SELECT record_id FROM map_saved_tombstone WHERE owner_account = :owner AND origin = :origin AND record_id IN (:ids)")
    suspend fun tombstoned(owner: String, origin: String, ids: List<String>): List<String>

    @Query("DELETE FROM map_saved_tombstone WHERE owner_account = :owner AND origin = :origin AND record_id IN (:ids)")
    suspend fun clearTombstones(owner: String, origin: String, ids: List<String>)

    @Query("SELECT COUNT(*) FROM map_saved_import WHERE owner_account = :owner AND origin = :origin AND content_sha256 = :sha")
    suspend fun importSeen(owner: String, origin: String, sha: String): Int

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun recordImport(row: SavedImportEntity)

    @Query("DELETE FROM map_saved_place WHERE owner_account = :owner AND origin = :origin")
    suspend fun clearPlaces(owner: String, origin: String)

    @Query("DELETE FROM map_saved_trip WHERE owner_account = :owner AND origin = :origin")
    suspend fun clearTrips(owner: String, origin: String)

    @Query("DELETE FROM map_saved_tombstone WHERE owner_account = :owner AND origin = :origin")
    suspend fun clearAllTombstones(owner: String, origin: String)

    @Query("DELETE FROM map_saved_import WHERE owner_account = :owner AND origin = :origin")
    suspend fun clearImports(owner: String, origin: String)
}
