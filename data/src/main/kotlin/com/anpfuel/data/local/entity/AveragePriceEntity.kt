package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.ForeignKey
import androidx.room.Index
import androidx.room.PrimaryKey

@Entity(
    tableName = "average_price",
    foreignKeys = [
        ForeignKey(
            entity = SurveyWeekEntity::class,
            parentColumns = ["id"],
            childColumns = ["survey_week_id"],
            onDelete = ForeignKey.CASCADE,
        ),
    ],
    indices = [
        Index(
            name = "idx_avg_survey_state_muni",
            value = ["survey_week_id", "state", "municipality"],
        ),
        Index(
            name = "idx_avg_muni_product",
            value = ["municipality", "fuel_product"],
        ),
        Index(
            value = ["survey_week_id", "state", "municipality", "fuel_product"],
            unique = true,
        ),
    ],
)
data class AveragePriceEntity(
    @PrimaryKey val id: String,
    @ColumnInfo(name = "survey_week_id") val surveyWeekId: String,
    val state: String,
    val municipality: String,
    @ColumnInfo(name = "fuel_product") val fuelProduct: String,
    @ColumnInfo(name = "station_count") val stationCount: Int?,
    val unit: String?,
    @ColumnInfo(name = "avg_price") val avgPrice: Double?,
    @ColumnInfo(name = "min_price") val minPrice: Double?,
    @ColumnInfo(name = "max_price") val maxPrice: Double?,
    @ColumnInfo(name = "std_dev") val stdDev: Double?,
)
