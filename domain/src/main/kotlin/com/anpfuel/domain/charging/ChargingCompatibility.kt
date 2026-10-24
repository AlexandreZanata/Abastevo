package com.anpfuel.domain.charging

/**
 * EE-21 compatibility of one public place with one vehicle.
 *
 * COMPATIBLE means at least one group carries the vehicle's connector;
 * UNKNOWN means no group confirms it (unknown connector, unknown
 * vehicle connector, or no groups at all) — unknown is shown, never
 * upgraded; INCOMPATIBLE means every known group mismatches. Equipment
 * counts (`reported_points` vs group rows) never filter: both numbers
 * stay visible instead of being reconciled away.
 */
enum class Compatibility { COMPATIBLE, UNKNOWN, INCOMPATIBLE }

object ChargingCompatibility {

    fun assess(
        vehicleConnector: ChargingConnector?,
        groups: List<ChargingConnectionGroup>,
    ): Compatibility {
        if (vehicleConnector == null || groups.isEmpty()) return Compatibility.UNKNOWN
        if (groups.any { it.connector == vehicleConnector }) return Compatibility.COMPATIBLE
        if (groups.any { it.connector == ChargingConnector.UNKNOWN }) return Compatibility.UNKNOWN
        return Compatibility.INCOMPATIBLE
    }

    /**
     * Exact filter match for Explore windows. A null filter accepts
     * everything; UNKNOWN access never satisfies a PUBLIC filter
     * (unverifiable access is not public access).
     */
    fun matches(
        place: ChargingPlace,
        connector: ChargingConnector?,
        current: ChargingCurrent?,
        access: ChargingAccess?,
    ): Boolean {
        if (connector != null && place.groups.none { it.connector == connector }) return false
        if (current != null && place.groups.none { it.current == current }) return false
        if (access != null && place.publicAccess != access) return false
        return true
    }
}
