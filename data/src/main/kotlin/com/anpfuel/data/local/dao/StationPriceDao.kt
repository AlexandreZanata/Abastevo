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

    /**
     * Bounded discovery window. An empty [search] matches every row;
     * callers pass LIKE-escaped text so `%`/`_` stay literal. Ordering
     * carries a stable `id` tiebreak so pages never shift or repeat
     * rows; `%` matches anything, hence the match-all fast path.
     */
    @Query(
        """
        SELECT * FROM station_price
        WHERE survey_week_id = :surveyWeekId
          AND state = :state
          AND municipality = :municipality
          AND fuel_product = :fuelProduct
          AND (:search = '' OR trade_name LIKE '%' || :search || '%' ESCAPE '\' COLLATE NOCASE
            OR legal_name LIKE '%' || :search || '%' ESCAPE '\' COLLATE NOCASE
            OR brand LIKE '%' || :search || '%' ESCAPE '\' COLLATE NOCASE
            OR address LIKE '%' || :search || '%' ESCAPE '\' COLLATE NOCASE)
        ORDER BY price ASC, id ASC
        LIMIT :limit OFFSET :offset
        """,
    )
    suspend fun findPageByPrice(
        surveyWeekId: String,
        state: String,
        municipality: String,
        fuelProduct: String,
        search: String,
        limit: Int,
        offset: Int,
    ): List<StationPriceEntity>

    @Query(
        """
        SELECT * FROM station_price
        WHERE survey_week_id = :surveyWeekId
          AND state = :state
          AND municipality = :municipality
          AND fuel_product = :fuelProduct
          AND (:search = '' OR trade_name LIKE '%' || :search || '%' ESCAPE '\' COLLATE NOCASE
            OR legal_name LIKE '%' || :search || '%' ESCAPE '\' COLLATE NOCASE
            OR brand LIKE '%' || :search || '%' ESCAPE '\' COLLATE NOCASE
            OR address LIKE '%' || :search || '%' ESCAPE '\' COLLATE NOCASE)
        ORDER BY collected_at IS NULL ASC, collected_at DESC, price ASC, id ASC
        LIMIT :limit OFFSET :offset
        """,
    )
    suspend fun findPageByRecency(
        surveyWeekId: String,
        state: String,
        municipality: String,
        fuelProduct: String,
        search: String,
        limit: Int,
        offset: Int,
    ): List<StationPriceEntity>

    @Query(
        """
        SELECT COUNT(*) FROM station_price
        WHERE survey_week_id = :surveyWeekId
          AND state = :state
          AND municipality = :municipality
          AND fuel_product = :fuelProduct
          AND (:search = '' OR trade_name LIKE '%' || :search || '%' ESCAPE '\' COLLATE NOCASE
            OR legal_name LIKE '%' || :search || '%' ESCAPE '\' COLLATE NOCASE
            OR brand LIKE '%' || :search || '%' ESCAPE '\' COLLATE NOCASE
            OR address LIKE '%' || :search || '%' ESCAPE '\' COLLATE NOCASE)
        """,
    )
    suspend fun countPage(
        surveyWeekId: String,
        state: String,
        municipality: String,
        fuelProduct: String,
        search: String,
    ): Int

    @Query(
        """
        SELECT * FROM station_price
        WHERE survey_week_id = :surveyWeekId
          AND state = :state
          AND municipality = :municipality
          AND fuel_product = :fuelProduct
          AND cnpj = :cnpjDigits
        LIMIT 1
        """,
    )
    suspend fun findByCnpj(
        surveyWeekId: String,
        state: String,
        municipality: String,
        fuelProduct: String,
        cnpjDigits: String,
    ): StationPriceEntity?

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
