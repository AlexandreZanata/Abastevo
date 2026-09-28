package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import com.anpfuel.data.local.entity.SurveyWeekEntity

@Dao
interface SurveyWeekDao {

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insert(entity: SurveyWeekEntity)

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insertAll(entities: List<SurveyWeekEntity>)

    @Query("SELECT * FROM survey_week WHERE id = :id LIMIT 1")
    suspend fun findById(id: String): SurveyWeekEntity?

    @Query("SELECT COUNT(*) FROM survey_week")
    suspend fun count(): Int

    @Query("UPDATE survey_week SET station_imported_at = :stationImportedAt WHERE id = :id")
    suspend fun updateStationImportedAt(id: String, stationImportedAt: Long)

    @Query("UPDATE survey_week SET station_imported_at = NULL WHERE id = :id")
    suspend fun clearStationImportedAt(id: String)

    @Query("SELECT * FROM survey_week ORDER BY end_date DESC")
    suspend fun findAllOrderedByEndDateDesc(): List<SurveyWeekEntity>

    @Query(
        """
        SELECT * FROM survey_week
        WHERE start_date = :startDate AND end_date = :endDate
        LIMIT 1
        """,
    )
    suspend fun findByDates(startDate: String, endDate: String): SurveyWeekEntity?

    @Query(
        """
        SELECT * FROM survey_week
        WHERE station_imported_at IS NOT NULL
        ORDER BY end_date DESC
        """,
    )
    suspend fun findAllWithStationDataOrderedByEndDateDesc(): List<SurveyWeekEntity>

    @Query("DELETE FROM survey_week")
    suspend fun deleteAll()
}
