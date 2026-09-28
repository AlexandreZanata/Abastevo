package com.anpfuel.domain.valueobject

/**
 * Ephemeral latitude/longitude from Android location APIs (UC-012, UC-015).
 * Not persisted as PII — used only for one-shot reverse geocoding (UC-012) and
 * one-shot nearest-station distance calculation (UC-015).
 */
class DeviceLocation private constructor(
    val coordinates: GeoCoordinates,
) {
    val latitude: Double get() = coordinates.latitude

    val longitude: Double get() = coordinates.longitude

    override fun equals(other: Any?): Boolean =
        other is DeviceLocation && coordinates == other.coordinates

    override fun hashCode(): Int = coordinates.hashCode()

    override fun toString(): String = "DeviceLocation(lat=$latitude, lon=$longitude)"

    companion object {
        fun of(latitude: Double, longitude: Double): DeviceLocation =
            DeviceLocation(GeoCoordinates.of(latitude, longitude))
    }
}
