package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity

/**
 * Room cache entity for Forum Abastevo (FOR-07 / B-BR-FOR-01..07).
 *
 * Caches categories, feed snapshots (up to 5 scopes), and post details
 * for offline reading with a 24-hour TTL and 2 MiB storage cap.
 * Scope key and origin prevent cross-origin cache leakage.
 */
@Entity(
    tableName = "forum_cache",
    primaryKeys = ["origin", "key"],
)
data class ForumCacheEntity(
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "key") val key: String,
    @ColumnInfo(name = "kind") val kind: String,
    @ColumnInfo(name = "payload_json") val payloadJson: String,
    @ColumnInfo(name = "fetched_at_millis") val fetchedAtMillis: Long,
    @ColumnInfo(name = "payload_bytes") val payloadBytes: Long,
)
