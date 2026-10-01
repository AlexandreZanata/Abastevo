package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.PrimaryKey

/**
 * P10-T05 durable outbox row (BUC-003, B-BR-003/005).
 *
 * Columns mirror [com.anpfuel.application.portable.PortableOutbox.toRecord]
 * field-for-field via [com.anpfuel.data.local.outbox.OutboxCommandMapper]:
 * one stable command id holds one pending intent; re-enqueue bumps
 * `revision` (attempts kept) so duplicate send/process death yields one
 * observation. `nonce` is the last dispatch nonce (fresh per send);
 * `next_eligible_tick` gates FAILED backoff. No contributor id, GPS, EXIF
 * or signed URL is stored here.
 */
@Entity(tableName = "contribution_outbox")
data class ContributionOutboxEntity(
    @PrimaryKey @ColumnInfo(name = "command_id") val commandId: String,
    @ColumnInfo(name = "kind") val kind: String,
    @ColumnInfo(name = "payload") val payload: String,
    @ColumnInfo(name = "revision") val revision: Int,
    @ColumnInfo(name = "state") val state: String,
    @ColumnInfo(name = "attempts") val attempts: Int,
    @ColumnInfo(name = "next_eligible_tick") val nextEligibleTick: Long,
    @ColumnInfo(name = "nonce") val nonce: String,
)
