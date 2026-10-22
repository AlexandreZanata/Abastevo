package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.PrimaryKey

/**
 * OPT private community snapshot (first page per feed scope, full summary
 * per city). One row holds one validated response payload with its original
 * dates; validity is decided at read time from the payload, never extended
 * here. Deeper cursor pages are intentionally not stored.
 */
@Entity(tableName = "community_feed_cache")
data class CommunityFeedCacheEntity(
    @PrimaryKey val key: String,
    @ColumnInfo(name = "kind") val kind: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "state") val state: String,
    @ColumnInfo(name = "municipality_code") val municipalityCode: String,
    @ColumnInfo(name = "fuel") val fuel: String?,
    @ColumnInfo(name = "sort") val sort: String?,
    @ColumnInfo(name = "payload_json") val payloadJson: String,
    @ColumnInfo(name = "generated_at_millis") val generatedAtMillis: Long,
    @ColumnInfo(name = "fetched_at_millis") val fetchedAtMillis: Long,
)
