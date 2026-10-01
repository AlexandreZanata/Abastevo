package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.PrimaryKey

/**
 * P10-T02 backend price-group cache (source/version/expiry explicit).
 *
 * Key is `stationId|fuelFilter` (`ALL` when unfiltered) so one row holds
 * one `GET /v1/stations/{id}/prices` response. Payload is the raw backend
 * JSON; ANP tables stay untouched.
 */
@Entity(tableName = "backend_price_cache")
data class BackendPriceCacheEntity(
    @PrimaryKey val key: String,
    @ColumnInfo(name = "station_id") val stationId: String,
    @ColumnInfo(name = "fuel_filter") val fuelFilter: String,
    @ColumnInfo(name = "payload_json") val payloadJson: String,
    @ColumnInfo(name = "source") val source: String,
    @ColumnInfo(name = "version") val version: String,
    @ColumnInfo(name = "fetched_at_millis") val fetchedAtMillis: Long,
    @ColumnInfo(name = "expires_at_millis") val expiresAtMillis: Long,
) {
    companion object {
        const val ALL_FUELS = "ALL"

        fun keyFor(stationId: String, fuelFilter: String?): String =
            "${stationId.lowercase()}|${fuelFilter ?: ALL_FUELS}"
    }
}
