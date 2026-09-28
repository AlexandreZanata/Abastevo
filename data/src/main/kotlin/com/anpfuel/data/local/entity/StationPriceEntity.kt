package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.ForeignKey
import androidx.room.Index
import androidx.room.PrimaryKey

@Entity(
    tableName = "station_price",
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
            name = "idx_station_survey_muni_product",
            value = ["survey_week_id", "municipality", "fuel_product"],
        ),
        Index(
            name = "idx_station_price_asc",
            value = ["survey_week_id", "municipality", "fuel_product", "price"],
        ),
    ],
)
data class StationPriceEntity(
    @PrimaryKey val id: String,
    @ColumnInfo(name = "survey_week_id") val surveyWeekId: String,
    val cnpj: String,
    @ColumnInfo(name = "legal_name") val legalName: String?,
    @ColumnInfo(name = "trade_name") val tradeName: String?,
    val address: String,
    val municipality: String,
    val state: String,
    val brand: String?,
    @ColumnInfo(name = "fuel_product") val fuelProduct: String,
    val price: Double,
    @ColumnInfo(name = "collected_at") val collectedAt: String?,
)
