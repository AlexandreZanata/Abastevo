package com.anpfuel.domain.rule

import com.anpfuel.domain.valueobject.GeoCoordinates
import kotlin.math.asin
import kotlin.math.cos
import kotlin.math.min
import kotlin.math.pow
import kotlin.math.sin
import kotlin.math.sqrt

/**
 * Great-circle (Haversine) distance between two [GeoCoordinates] in meters (UC-015).
 */
object GeoDistanceRule {

    private const val EARTH_RADIUS_METERS = 6_371_008.8

    fun distanceMeters(from: GeoCoordinates, to: GeoCoordinates): Double {
        val fromLatRadians = Math.toRadians(from.latitude)
        val toLatRadians = Math.toRadians(to.latitude)
        val deltaLatRadians = Math.toRadians(to.latitude - from.latitude)
        val deltaLonRadians = Math.toRadians(to.longitude - from.longitude)

        val haversine = sin(deltaLatRadians / 2).pow(2) +
            cos(fromLatRadians) * cos(toLatRadians) * sin(deltaLonRadians / 2).pow(2)

        return 2 * EARTH_RADIUS_METERS * asin(min(1.0, sqrt(haversine)))
    }
}
