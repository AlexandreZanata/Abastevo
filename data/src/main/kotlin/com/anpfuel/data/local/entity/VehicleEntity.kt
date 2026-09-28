package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.PrimaryKey

@Entity(tableName = "vehicle")
data class VehicleEntity(
    @PrimaryKey val id: String,
    @ColumnInfo(name = "display_name") val displayName: String,
    @ColumnInfo(name = "tank_capacity_liters") val tankCapacityLiters: Double,
    @ColumnInfo(name = "fuel_product") val fuelProduct: String,
    @ColumnInfo(name = "price_source_mode") val priceSourceMode: String,
    @ColumnInfo(name = "specific_station_cnpj") val specificStationCnpj: String?,
    @ColumnInfo(name = "price_drop_alert_enabled") val priceDropAlertEnabled: Boolean,
    @ColumnInfo(name = "sort_order") val sortOrder: Int,
)
