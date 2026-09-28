package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import com.anpfuel.data.local.entity.StationPriceEntity

@Dao
interface StationPriceDao {

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insertAll(entities: List<StationPriceEntity>)

    @Query(
        """
        SELECT * FROM station_price
        WHERE survey_week_id = :surveyWeekId
          AND state = :state
          AND municipality = :municipality
        ORDER BY price ASC
        """,
    )
    suspend fun findByLocationOrderedByPrice(
        surveyWeekId: String,
        state: String,
        municipality: String,
    ): List<StationPriceEntity>

    @Query("SELECT COUNT(*) FROM station_price")
    suspend fun count(): Int

    @Query(
        """
        SELECT * FROM station_price
        WHERE survey_week_id = :surveyWeekId
          AND state = :state
          AND municipality = :municipality
          AND fuel_product = :fuelProduct
        ORDER BY price ASC
        """,
    )
    suspend fun findByLocationAndProduct(
        surveyWeekId: String,
        state: String,
        municipality: String,
        fuelProduct: String,
    ): List<StationPriceEntity>

    @Query(
        """
        SELECT COUNT(*) FROM station_price
        WHERE survey_week_id = :surveyWeekId
          AND state = :state
          AND municipality = :municipality
        """,
    )
    suspend fun countByLocation(
        surveyWeekId: String,
        state: String,
        municipality: String,
    ): Int

    @Query("DELETE FROM station_price WHERE survey_week_id IN (:surveyWeekIds)")
    suspend fun deleteBySurveyWeekIds(surveyWeekIds: List<String>)

    @Query("DELETE FROM station_price")
    suspend fun deleteAll()
}
