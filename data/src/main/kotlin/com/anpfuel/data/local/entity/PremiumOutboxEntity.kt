package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.Index

/**
 * PRM09-T01 durable premium command row.
 *
 * One stable `command_id` holds one pending intent; re-enqueue keeps the
 * same id so duplicate save/process death converges to one effect. Every
 * row is bound to `owner_account` + `origin`: account/origin switches can
 * never read or replay another owner's queue. No GPS, photos, credentials
 * or provider tokens are stored here; payloads are typed dataset JSON.
 */
@Entity(
    tableName = "premium_outbox",
    primaryKeys = ["command_id"],
    indices = [
        Index(value = ["owner_account", "state", "next_eligible_tick"]),
        Index(value = ["owner_account", "origin", "dataset"]),
    ],
)
data class PremiumOutboxEntity(
    @ColumnInfo(name = "command_id") val commandId: String,
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "dataset") val dataset: String,
    @ColumnInfo(name = "record_id") val recordId: String,
    @ColumnInfo(name = "expected_revision") val expectedRevision: Long,
    @ColumnInfo(name = "payload_json") val payloadJson: String,
    @ColumnInfo(name = "delete_flag") val deleteFlag: Boolean,
    @ColumnInfo(name = "void_flag") val voidFlag: Boolean,
    @ColumnInfo(name = "state") val state: String,
    @ColumnInfo(name = "attempts") val attempts: Int,
    @ColumnInfo(name = "next_eligible_tick") val nextEligibleTick: Long,
    @ColumnInfo(name = "nonce") val nonce: String,
)
