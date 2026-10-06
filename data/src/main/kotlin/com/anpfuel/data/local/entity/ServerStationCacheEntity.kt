package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.PrimaryKey

/**
 * P27-T03 canonical server catalog cache.
 *
 * One row per canonical Directory UUID from `GET /v1/stations`
 * traversal; zero-price stations stay visible offline. Coordinates
 * appear only when the server supplied them (honest unknown stays
 * null); no centroid is ever fabricated here. `position` preserves the
 * server page order so pagination/refresh stay deterministic.
 */
@Entity(tableName = "server_station_cache")
data class ServerStationCacheEntity(
    @PrimaryKey @ColumnInfo(name = "station_id") val stationId: String,
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
)

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
