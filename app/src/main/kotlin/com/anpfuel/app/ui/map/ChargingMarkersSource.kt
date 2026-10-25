package com.anpfuel.app.ui.map

import com.anpfuel.domain.charging.ChargingAccess
import com.anpfuel.domain.charging.ChargingCompatibility
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingCurrent
import com.anpfuel.domain.charging.ChargingPlace
import kotlin.math.cos
import kotlin.math.sqrt

data class ChargingMarkersQuery(
    val latitude: Double? = null,
    val longitude: Double? = null,
    val radiusMeters: Int = 10_000,
    val searchQuery: String? = null,
    val connector: ChargingConnector? = null,
    val current: ChargingCurrent? = null,
    val access: ChargingAccess? = null,
    val minPowerKw: Int? = null,
    val limit: Int = 100,
)

data class ChargingMarkersResult(
    val places: List<ChargingPlace>,
    val snapshotPresent: Boolean = true,
    val error: String? = null,
)

/**
 * Narrow source for charging markers. The product implementation reads the
 * offline EE-18 snapshot across pages with region/query/filter matching,
 * rethrows CancellationException, and preserves snapshot/error state.
 */
fun interface ChargingMarkersSource {
    suspend fun list(limit: Int): List<ChargingPlace>

    suspend fun query(request: ChargingMarkersQuery): ChargingMarkersResult {
        val raw = list(request.limit.coerceAtLeast(100))
        val filtered = raw.filter { place ->
            matchesChargingQuery(place, request)
        }.take(request.limit.coerceIn(1, 200))
        return ChargingMarkersResult(places = filtered, snapshotPresent = true, error = null)
    }
}

internal fun matchesChargingQuery(place: ChargingPlace, query: ChargingMarkersQuery): Boolean {
    if (!ChargingCompatibility.matches(place, query.connector, query.current, query.access)) {
        return false
    }
    val minKw = query.minPowerKw
    if (minKw != null && minKw > 0) {
        val minW = minKw.toLong() * 1_000L
        // Unknown power (groups empty or powerW == null) stays visible per B-BR-MAP-FILTERS-01.
        val powerMatches = place.groups.isEmpty() ||
            place.groups.any { group ->
                val pw = group.powerW
                pw == null || pw >= minW
            }
        if (!powerMatches) return false
    }
    val q = query.searchQuery?.trim()
    if (!q.isNullOrEmpty()) {
        val textMatch = place.name.contains(q, ignoreCase = true) ||
            place.externalId.contains(q, ignoreCase = true)
        if (!textMatch) return false
    }
    val centerLat = query.latitude
    val centerLon = query.longitude
    if (centerLat != null && centerLon != null && q.isNullOrEmpty()) {
        val pLat = place.latitude?.toDoubleOrNull()
        val pLon = place.longitude?.toDoubleOrNull()
        if (pLat != null && pLon != null) {
            val dist = approxDistanceMeters(centerLat, centerLon, pLat, pLon)
            if (dist > query.radiusMeters.coerceIn(100, 50_000).toDouble()) {
                return false
            }
        }
    }
    return true
}

private fun approxDistanceMeters(lat1: Double, lon1: Double, lat2: Double, lon2: Double): Double {
    val x = Math.toRadians(lon2 - lon1) * cos(Math.toRadians((lat1 + lat2) / 2.0))
    val y = Math.toRadians(lat2 - lat1)
    return sqrt(x * x + y * y) * 6_371_000.0
}
