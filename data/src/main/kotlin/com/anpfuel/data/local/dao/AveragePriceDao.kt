package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import com.anpfuel.data.local.entity.AveragePriceEntity
import com.anpfuel.data.local.model.MunicipalityLocationRow

@Dao
interface AveragePriceDao {

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insertAll(entities: List<AveragePriceEntity>)

    @Query(
        """
        SELECT * FROM average_price
        WHERE survey_week_id = :surveyWeekId
          AND state = :state
          AND municipality = :municipality
        """,
    )
    suspend fun findByLocation(
        surveyWeekId: String,
        state: String,
        municipality: String,
    ): List<AveragePriceEntity>

    @Query("SELECT COUNT(*) FROM average_price")
    suspend fun count(): Int

    @Query("SELECT * FROM average_price WHERE survey_week_id = :surveyWeekId LIMIT 1")
    suspend fun findAnyBySurveyWeek(surveyWeekId: String): AveragePriceEntity?

    @Query(
        """
        SELECT ap.* FROM average_price ap
        INNER JOIN survey_week sw ON ap.survey_week_id = sw.id
        WHERE ap.state = :state
          AND ap.municipality = :municipality
          AND ap.fuel_product = :fuelProduct
        ORDER BY sw.end_date ASC
        """,
    )
    suspend fun findPriceHistory(
        state: String,
        municipality: String,
        fuelProduct: String,
    ): List<AveragePriceEntity>

    @Query(
        """
        SELECT DISTINCT state FROM average_price
        WHERE survey_week_id = :surveyWeekId
        ORDER BY state ASC
        """,
    )
    suspend fun findDistinctStates(surveyWeekId: String): List<String>

    @Query(
        """
        SELECT DISTINCT municipality FROM average_price
        WHERE survey_week_id = :surveyWeekId AND state = :state
        ORDER BY municipality COLLATE NOCASE ASC
        """,
    )
    suspend fun findDistinctMunicipalities(
        surveyWeekId: String,
        state: String,
    ): List<String>

    @Query(
        """
        SELECT DISTINCT state, municipality FROM average_price
        WHERE survey_week_id = :surveyWeekId
        """,
    )
    suspend fun findDistinctLocationsBySurveyWeek(surveyWeekId: String): List<MunicipalityLocationRow>

    @Query(
        """
        SELECT DISTINCT state, municipality FROM average_price
        """,
    )
    suspend fun findDistinctLocationsEverImported(): List<MunicipalityLocationRow>

    @Query("DELETE FROM average_price")
    suspend fun deleteAll()
}
