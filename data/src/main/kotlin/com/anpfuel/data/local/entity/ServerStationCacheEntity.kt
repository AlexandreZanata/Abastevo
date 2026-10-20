package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.PrimaryKey

/**
 * P27-T03 canonical server catalog cache.
 *
 * Identity is the Directory UUID **plus** the owning scope: page
 * membership rows carry their scope (`"page"` nationally,
 * `"city:<ibge>"` per municipality) and `position` preserves the
 * server order so pagination/refresh stay deterministic, while
 * individually fetched details live under the unscoped detail scope
 * with `position = Int.MAX_VALUE` and are never cleared by page
 * replacement. Station records therefore survive beside — never
 * inside — page participation. Coordinates appear only when the
 * server supplied them (honest unknown stays null); no centroid is
 * ever fabricated here. Zero-price stations stay visible offline.
 */
@Entity(
    tableName = "server_station_cache",
    primaryKeys = ["station_id", "scope_key"],
    indices = [androidx.room.Index(value = ["scope_key", "position"])],
)
data class ServerStationCacheEntity(
    @ColumnInfo(name = "station_id") val stationId: String,
    @ColumnInfo(name = "display_name") val displayName: String,
    @ColumnInfo(name = "location_quality") val locationQuality: String,
    @ColumnInfo(name = "latitude") val latitude: Double?,
    @ColumnInfo(name = "longitude") val longitude: Double?,
    @ColumnInfo(name = "cnpj_normalized") val cnpjNormalized: String?,
    @ColumnInfo(name = "municipality_code") val municipalityCode: String?,
    @ColumnInfo(name = "state") val state: String?,
    @ColumnInfo(name = "current_revision_id") val currentRevisionId: String?,
    @ColumnInfo(name = "position") val position: Int,
    @ColumnInfo(name = "saved_at_millis") val savedAtMillis: Long,
    @ColumnInfo(name = "address_json", defaultValue = "'{}'") val addressJson: String = "{}",
    @ColumnInfo(name = "scope_key", defaultValue = "''") val scopeKey: String = "",
) {
    companion object {
        /** Unscoped identity records (details); page replacement never deletes them. */
        const val DETAIL_SCOPE = ""
    }
}

/**
 * P27-T03 single-row page cursor for the cached catalog (`key` is
 * always `"page"`). A null cursor ends traversal; absence means no
 * cached page.
 */
@Entity(tableName = "server_catalog_meta")
data class ServerCatalogMetaEntity(
    @PrimaryKey @ColumnInfo(name = "key") val key: String,
    @ColumnInfo(name = "next_cursor") val nextCursor: String?,
    @ColumnInfo(name = "saved_at_millis") val savedAtMillis: Long,
) {
    companion object {
        const val PAGE_KEY = "page"
    }
}
