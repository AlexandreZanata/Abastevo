package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.Index
import androidx.room.PrimaryKey

/**
 * EE-18 public EV snapshot rows: one validated charging place per
 * generation. Identity is (place_id, generation); a resnapshot writes a
 * fresh generation and deletes the old one atomically, so readers never
 * see a half-written pack. Retired places stay as tombstone rows (payload
 * dropped, identity kept) until the next resnapshot clears them; reads
 * filter tombstones out. Coordinates stay null when the server did not
 * supply them; no centroid is ever fabricated. Fuel, history and private
 * tables are never referenced here.
 */
@Entity(
    tableName = "charging_snapshot_place",
    primaryKeys = ["place_id", "generation"],
    indices = [
        Index(value = ["generation"]),
        Index(value = ["profile", "generation"]),
    ],
)
data class ChargingSnapshotPlaceEntity(
    @ColumnInfo(name = "place_id") val placeId: String,
    @ColumnInfo(name = "generation") val generation: Long,
    @ColumnInfo(name = "profile") val profile: String,
    @ColumnInfo(name = "source_revision") val sourceRevision: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "license") val license: String,
    @ColumnInfo(name = "name") val name: String,
    @ColumnInfo(name = "country") val country: String,
    @ColumnInfo(name = "latitude") val latitude: String?,
    @ColumnInfo(name = "longitude") val longitude: String?,
    @ColumnInfo(name = "public_access") val publicAccess: String,
    @ColumnInfo(name = "reported_points") val reportedPoints: Long,
    @ColumnInfo(name = "groups_json") val groupsJson: String,
    @ColumnInfo(name = "tombstone") val tombstone: Boolean,
    @ColumnInfo(name = "saved_at_millis") val savedAtMillis: Long,
)

/**
 * EE-18 snapshot generation metadata: one row per pack key
 * (`"pack:<profile>"`). State is IN_PROGRESS while deltas land and
 * COMPLETE once the manifest commits; cursor holds the resume point of
 * an interrupted delta. A resnapshot with a new base deletes the old
 * generation rows in the same transaction that swaps this row.
 */
@Entity(tableName = "charging_snapshot_meta")
data class ChargingSnapshotMetaEntity(
    @PrimaryKey @ColumnInfo(name = "key") val key: String,
    @ColumnInfo(name = "generation") val generation: Long,
    @ColumnInfo(name = "profile") val profile: String,
    @ColumnInfo(name = "source_revision") val sourceRevision: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "license") val license: String,
    @ColumnInfo(name = "schema_version") val schemaVersion: Int,
    @ColumnInfo(name = "base_generation") val baseGeneration: Long,
    @ColumnInfo(name = "place_count") val placeCount: Long,
    @ColumnInfo(name = "group_count") val groupCount: Long,
    @ColumnInfo(name = "cursor") val cursor: String?,
    @ColumnInfo(name = "state") val state: String,
    @ColumnInfo(name = "saved_at_millis") val savedAtMillis: Long,
) {
    companion object {
        const val STATE_IN_PROGRESS = "IN_PROGRESS"
        const val STATE_COMPLETE = "COMPLETE"

        fun packKey(profile: String): String = "pack:$profile"
    }
}
