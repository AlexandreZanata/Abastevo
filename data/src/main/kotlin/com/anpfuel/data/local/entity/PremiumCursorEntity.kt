package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity

/**
 * PRM09-T01 opaque cursor row.
 *
 * Cursors are HMAC strings minted by the backend; the client returns them
 * verbatim until empty and never parses them. One row per owner/origin/
 * dataset keeps two accounts and two origins isolated on shared devices.
 */
@Entity(
    tableName = "premium_cursor",
    primaryKeys = ["owner_account", "origin", "dataset"],
)
data class PremiumCursorEntity(
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "dataset") val dataset: String,
    @ColumnInfo(name = "cursor") val cursor: String,
    @ColumnInfo(name = "epoch") val epoch: Long,
    @ColumnInfo(name = "updated_at_millis") val updatedAtMillis: Long,
)
