package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.PrimaryKey

@Entity(tableName = "import_audit_log")
data class ImportAuditLogEntity(
    @PrimaryKey val id: String,
    @ColumnInfo(name = "survey_week_id") val surveyWeekId: String?,
    val action: String,
    val detail: String?,
    @ColumnInfo(name = "occurred_at") val occurredAt: Long,
)
