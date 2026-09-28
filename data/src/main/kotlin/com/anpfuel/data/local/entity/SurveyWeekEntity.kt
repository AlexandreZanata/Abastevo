package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.PrimaryKey

@Entity(tableName = "survey_week")
data class SurveyWeekEntity(
    @PrimaryKey val id: String,
    @ColumnInfo(name = "start_date") val startDate: String,
    @ColumnInfo(name = "end_date") val endDate: String,
    @ColumnInfo(name = "summary_imported_at") val summaryImportedAt: Long,
    @ColumnInfo(name = "station_imported_at") val stationImportedAt: Long?,
)
