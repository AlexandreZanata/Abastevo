package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import androidx.room.Transaction
import com.anpfuel.data.local.entity.PhotoUploadSessionEntity
import com.anpfuel.domain.exception.DomainException

@Dao
interface PhotoUploadSessionDao {
    @Query("SELECT * FROM photo_upload_sessions WHERE capture_id=:captureId LIMIT 1")
    suspend fun find(captureId: String): PhotoUploadSessionEntity?

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insert(session: PhotoUploadSessionEntity): Long

    @Transaction
    suspend fun remember(session: PhotoUploadSessionEntity): PhotoUploadSessionEntity {
        insert(session)
        val saved = find(session.captureId) ?: throw DomainException("photo negotiation was not saved")
        if (saved.copy(evidenceId = null) != session.copy(evidenceId = null)) {
            throw DomainException("photo negotiation changed")
        }
        return saved
    }

    @Query("""UPDATE photo_upload_sessions SET evidence_id=:evidenceId WHERE capture_id=:captureId
        AND session_id=:sessionId AND (evidence_id IS NULL OR evidence_id=:evidenceId)""")
    suspend fun ready(captureId: String, sessionId: String, evidenceId: String): Int
}
