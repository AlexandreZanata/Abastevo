package com.anpfuel.domain.map.saved

import com.anpfuel.domain.routing.NavigationFix
import com.anpfuel.domain.routing.NavigationFixPolicy
import kotlin.math.asin
import kotlin.math.cos
import kotlin.math.pow
import kotlin.math.sin
import kotlin.math.sqrt

/**
 * MAP-25 B-BR-MAP25-05: which fixes become track points. Same quality gate as guidance
 * (finite, accuracy <= 50 m, fresh); a point is kept only when it moved at least
 * [MIN_STEP_METRES] from the previous kept point, so a stationary phone does not fill the
 * 10,000-point limit and no raw high-rate stream is stored.
 */
object TrackRecordingPolicy {
    const val MIN_STEP_METRES = 10.0

    fun accept(fix: NavigationFix, nowNanos: Long, previous: TripPoint?): TripPoint? {
        if (!NavigationFixPolicy.accepts(fix, nowNanos)) return null
        val point = try { GeoPoint.fromDouble(fix.latitude, fix.longitude) } catch (_: SavedValidationException) { return null }
        if (previous != null && metres(previous.point, point) < MIN_STEP_METRES) return null
        val elevation = if (fix.altitudeMeters.isFinite()) (fix.altitudeMeters * 100).toLong().takeIf { it in -1_000_000..10_000_000 }?.toInt() else null
        return TripPoint(point, elevation, fix.timeMillis)
    }

    fun metres(a: GeoPoint, b: GeoPoint): Double {
        val r = 6_371_008.8
        val p1 = Math.toRadians(a.latitude)
        val p2 = Math.toRadians(b.latitude)
        val dp = p2 - p1
        val dl = Math.toRadians(b.longitude - a.longitude)
        val h = sin(dp / 2).pow(2) + cos(p1) * cos(p2) * sin(dl / 2).pow(2)
        return 2 * r * asin(sqrt(h))
    }
}
