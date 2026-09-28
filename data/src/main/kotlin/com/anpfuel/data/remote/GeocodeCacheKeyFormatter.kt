package com.anpfuel.data.remote

import com.anpfuel.domain.valueobject.DeviceLocation
import kotlin.math.pow
import kotlin.math.round

object GeocodeCacheKeyFormatter {

    private const val DECIMAL_PLACES = 3

    fun format(location: DeviceLocation): String =
        "${roundCoordinate(location.latitude)},${roundCoordinate(location.longitude)}"

    fun roundCoordinate(value: Double): Double {
        val factor = 10.0.pow(DECIMAL_PLACES)
        return round(value * factor) / factor
    }
}
