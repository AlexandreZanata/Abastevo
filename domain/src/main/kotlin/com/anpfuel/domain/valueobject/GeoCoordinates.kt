package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException

/**
 * A validated point on Earth in WGS-84 decimal degrees (UC-015).
 */
class GeoCoordinates private constructor(
    val latitude: Double,
    val longitude: Double,
) {
    init {
        if (latitude !in MIN_LATITUDE..MAX_LATITUDE) {
            throw DomainException("GeoCoordinates latitude must be between $MIN_LATITUDE and $MAX_LATITUDE")
        }
        if (longitude !in MIN_LONGITUDE..MAX_LONGITUDE) {
            throw DomainException("GeoCoordinates longitude must be between $MIN_LONGITUDE and $MAX_LONGITUDE")
        }
    }

    override fun equals(other: Any?): Boolean =
        other is GeoCoordinates &&
            latitude == other.latitude &&
            longitude == other.longitude

    override fun hashCode(): Int = 31 * latitude.hashCode() + longitude.hashCode()

    override fun toString(): String = "GeoCoordinates(lat=$latitude, lon=$longitude)"

    companion object {
        const val MIN_LATITUDE = -90.0
        const val MAX_LATITUDE = 90.0
        const val MIN_LONGITUDE = -180.0
        const val MAX_LONGITUDE = 180.0

        fun of(latitude: Double, longitude: Double): GeoCoordinates =
            GeoCoordinates(latitude, longitude)
    }
}
