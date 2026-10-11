package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.Index
import androidx.room.PrimaryKey

/** One shared, immutable negotiation; private URLs/bytes never enter Room. */
@Entity(
    tableName = "photo_upload_sessions",
    indices = [
        // SEC06-C owner index: owner-scoped list/purge for erasure.
        Index(value = ["owner_scope"]),
    ],
)
data class PhotoUploadSessionEntity(
    @PrimaryKey @ColumnInfo(name = "capture_id") val captureId: String,
    @ColumnInfo(name = "session_id") val sessionId: String,
    @ColumnInfo(name = "photo_id") val photoId: String,
    @ColumnInfo(name = "owner_scope") val ownerScope: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "captured_at_millis") val capturedAtMillis: Long,
    @ColumnInfo(name = "expires_at_millis") val expiresAtMillis: Long,
    @ColumnInfo(name = "sha256") val sha256: String,
    @ColumnInfo(name = "evidence_id") val evidenceId: String? = null,
)
