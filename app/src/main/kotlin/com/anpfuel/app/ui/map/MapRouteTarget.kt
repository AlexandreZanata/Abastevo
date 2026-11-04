package com.anpfuel.app.ui.map

import com.anpfuel.domain.routing.RoutePoint

/** A local handoff from a station action. Address matches require user selection. */
sealed interface MapRouteTarget {
    data class Point(val point: RoutePoint) : MapRouteTarget
    data class Address(val query: String) : MapRouteTarget

    companion object {
        fun fromStationQuery(query: String, name: String): MapRouteTarget? {
            val value = query.trim()
            if (value.isEmpty()) return null
            val parts = value.split(',', limit = 3)
            val latitude = parts.getOrNull(0)?.trim()?.toDoubleOrNull()
            val longitude = parts.getOrNull(1)?.trim()?.toDoubleOrNull()
            if (parts.size == 2 && latitude != null && longitude != null) {
                return if (latitude in -90.0..90.0 && longitude in -180.0..180.0) {
                    Point(RoutePoint(latitude, longitude, name.take(512)))
                } else {
                    null
                }
            }
            return Address(value.take(256))
        }
    }
}
