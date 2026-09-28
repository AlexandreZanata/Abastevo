package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import com.anpfuel.data.local.entity.ImportAuditLogEntity

@Dao
interface ImportAuditLogDao {

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insert(entity: ImportAuditLogEntity)

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insertAll(entities: List<ImportAuditLogEntity>)

    @Query("SELECT COUNT(*) FROM import_audit_log")
    suspend fun count(): Int

    @Query("SELECT * FROM import_audit_log WHERE survey_week_id = :surveyWeekId ORDER BY occurred_at ASC")
    suspend fun findBySurveyWeek(surveyWeekId: String): List<ImportAuditLogEntity>

    @Query("DELETE FROM import_audit_log")
    suspend fun deleteAll()
}
