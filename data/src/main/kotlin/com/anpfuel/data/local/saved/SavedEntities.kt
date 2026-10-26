package com.anpfuel.data.local.saved

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.ForeignKey
import androidx.room.Index

/**
 * MAP-25 private favorites and saved trips. Every row is bound to owner + origin; nothing
 * here is synced, uploaded or placed in the SDK bookmark store (B-BR-MAP25-01/03).
 */
@Entity(
    tableName = "map_saved_place",
    primaryKeys = ["owner_account", "origin", "place_id"],
    indices = [
        Index(value = ["owner_account", "origin", "station_id"], unique = true, name = "index_map_saved_place_station"),
    ],
)
data class SavedPlaceEntity(
    @ColumnInfo(name = "place_id") val placeId: String,
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "kind") val kind: String,
    @ColumnInfo(name = "name") val name: String,
    @ColumnInfo(name = "lat_micro") val latMicro: Long,
    @ColumnInfo(name = "lon_micro") val lonMicro: Long,
    @ColumnInfo(name = "station_id") val stationId: String?,
    @ColumnInfo(name = "source_kind") val sourceKind: String,
    @ColumnInfo(name = "source_id") val sourceId: String?,
    @ColumnInfo(name = "note") val note: String?,
    @ColumnInfo(name = "created_at_millis") val createdAtMillis: Long,
)

/** `state` is COMPLETE or RECORDING (an explicit recording not yet saved or discarded). */
@Entity(
    tableName = "map_saved_trip",
    primaryKeys = ["owner_account", "origin", "trip_id"],
    indices = [Index(value = ["owner_account", "origin", "state"], name = "index_map_saved_trip_owner")],
)
data class SavedTripEntity(
    @ColumnInfo(name = "trip_id") val tripId: String,
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "state") val state: String,
    @ColumnInfo(name = "kind") val kind: String,
    @ColumnInfo(name = "name") val name: String,
    @ColumnInfo(name = "avoid") val avoid: String,
    @ColumnInfo(name = "source_id") val sourceId: String?,
    @ColumnInfo(name = "note") val note: String?,
    @ColumnInfo(name = "point_count") val pointCount: Int,
    @ColumnInfo(name = "created_at_millis") val createdAtMillis: Long,
)

@Entity(
    tableName = "map_saved_trip_point",
    primaryKeys = ["owner_account", "origin", "trip_id", "seq"],
    foreignKeys = [
        ForeignKey(
            entity = SavedTripEntity::class,
            parentColumns = ["owner_account", "origin", "trip_id"],
            childColumns = ["owner_account", "origin", "trip_id"],
            onDelete = ForeignKey.CASCADE,
        ),
    ],
)
data class SavedTripPointEntity(
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "trip_id") val tripId: String,
    @ColumnInfo(name = "seq") val seq: Int,
    @ColumnInfo(name = "lat_micro") val latMicro: Long,
    @ColumnInfo(name = "lon_micro") val lonMicro: Long,
    @ColumnInfo(name = "elevation_cm") val elevationCm: Int?,
    @ColumnInfo(name = "time_millis") val timeMillis: Long?,
)

/** Id only (no name, position or note): stops an import from silently resurrecting a deleted record. */
@Entity(tableName = "map_saved_tombstone", primaryKeys = ["owner_account", "origin", "record_id"])
data class SavedTombstoneEntity(
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "record_id") val recordId: String,
    @ColumnInfo(name = "deleted_at_millis") val deletedAtMillis: Long,
)

@Entity(tableName = "map_saved_import", primaryKeys = ["owner_account", "origin", "content_sha256"])
data class SavedImportEntity(
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "content_sha256") val contentSha256: String,
    @ColumnInfo(name = "imported_at_millis") val importedAtMillis: Long,
)
