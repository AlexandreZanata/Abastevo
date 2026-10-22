package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.Index

/**
 * MEX-11 durable ledger draft: one validated, unsent owner entry.
 *
 * Drafts are pre-outbox and user-editable; saving the same record twice
 * replaces the row (REPLACE on the stable draft id), so double-save
 * converges to one draft. Every row is bound to owner + origin: account
 * switches never read or resume another owner's drafts. Payloads carry
 * money/odometer only — no GPS, photos, credentials or provider tokens.
 */
@Entity(
    tableName = "ledger_draft",
    primaryKeys = ["draft_id"],
    indices = [
        Index(value = ["owner_account", "origin", "dataset", "record_id"], name = "index_ledger_draft_owner_record"),
    ],
)
data class LedgerDraftEntity(
    @ColumnInfo(name = "draft_id") val draftId: String,
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "dataset") val dataset: String,
    @ColumnInfo(name = "record_id") val recordId: String,
    @ColumnInfo(name = "expected_revision") val expectedRevision: Long,
    @ColumnInfo(name = "body_json") val bodyJson: String,
    @ColumnInfo(name = "updated_at_millis") val updatedAtMillis: Long,
    /**
     * EE-20 local-only session context (home/work/public/other). Never
     * synced: command payloads are built from the body alone.
     */
    @ColumnInfo(name = "context") val context: String? = null,
)
