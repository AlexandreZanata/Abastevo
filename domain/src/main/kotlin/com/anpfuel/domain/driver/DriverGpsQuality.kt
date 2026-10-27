package com.anpfuel.domain.driver

import kotlin.math.*

/** Ephemeral synthetic/device fix. Never a server payload or a log representation. */
class DriverGpsFix(val latitude: Double, val longitude: Double, val accuracyMetres: Double, val clock: DriverClock) {
    override fun toString() = "DriverGpsFix(private)"
}
data class DriverGpsPolicy(val version: Int, val maximumAccuracyMetres: Double, val maximumAgeMillis: Long,
                           val maximumGapMillis: Long, val maximumSpeedMetresPerSecond: Double, val driftBandMetres: Double) {
    init {
        require(version > 0 && maximumAccuracyMetres.isFinite() && maximumAccuracyMetres > 0 && maximumAgeMillis >= 0 && maximumGapMillis > 0)
        require(maximumSpeedMetresPerSecond.isFinite() && maximumSpeedMetresPerSecond > 0 && driftBandMetres.isFinite() && driftBandMetres >= 0)
    }
}
data class DriverDistanceScope(val owner: String, val origin: String, val vehicle: String, val session: String) {
    init { require(listOf(owner, origin, vehicle, session).all { it.isNotBlank() }) }
}
data class DriverGpsEstimate(val distanceMetres: Long, val policyVersion: Int, val acceptedFixes: Long,
                             val rejected: Map<String, Long>, val hasGap: Boolean) {
    val complete: Boolean get() = !hasGap && rejected.isEmpty()
}
/** Pure immutable reducer. It consumes fixes; it cannot obtain location or start recording. */
class DriverGpsQuality private constructor(
    val scope: DriverDistanceScope, val policy: DriverGpsPolicy,
    private val last: DriverGpsFix?, private val anchor: DriverGpsFix?, private val total: Double,
    private val accepted: Long, private val rejected: Map<String, Long>, private val gap: Boolean,
) {
    companion object {
        fun empty(scope: DriverDistanceScope, policy: DriverGpsPolicy) = DriverGpsQuality(scope, policy, null, null, 0.0, 0, emptyMap(), false)
        // Versioned spherical estimate, not a geodetic/odometer measurement.
        private fun separation(a: DriverGpsFix, b: DriverGpsFix): Double {
            val dLat = Math.toRadians(b.latitude - a.latitude); val dLon = Math.toRadians(b.longitude - a.longitude)
            val h = sin(dLat / 2).pow(2) + cos(Math.toRadians(a.latitude)) * cos(Math.toRadians(b.latitude)) * sin(dLon / 2).pow(2)
            return 6_371_000.0 * 2 * asin(sqrt(h.coerceIn(0.0, 1.0)))
        }
    }
    fun estimate() = DriverGpsEstimate(floor(total).toLong(), policy.version, accepted, rejected.toMap(), gap)
    fun append(fix: DriverGpsFix, received: DriverClock): DriverGpsQuality {
        fun refuse(reason: String) = DriverGpsQuality(scope, policy, last, anchor, total, accepted,
            rejected + (reason to Math.addExact(rejected.getOrDefault(reason, 0), 1)), gap || (last != null && last.clock.bootId != received.bootId))
        if (!fix.latitude.isFinite() || !fix.longitude.isFinite() || fix.latitude !in -90.0..90.0 || fix.longitude !in -180.0..180.0 ||
            !fix.accuracyMetres.isFinite() || fix.accuracyMetres < 0) return refuse("invalid-fix")
        if (fix.clock.bootId != received.bootId) return refuse("fix-boot-mismatch")
        if (fix.clock.elapsedMillis > received.elapsedMillis) return refuse("future-fix")
        if (received.elapsedMillis - fix.clock.elapsedMillis > policy.maximumAgeMillis) return refuse("stale-fix")
        if (fix.accuracyMetres > policy.maximumAccuracyMetres) return refuse("poor-accuracy")
        val count = Math.addExact(accepted, 1)
        if (last == null) return DriverGpsQuality(scope, policy, fix, fix, total, count, rejected, gap)
        if (last.clock.bootId != fix.clock.bootId) return DriverGpsQuality(scope, policy, fix, fix, total, count, rejected, true)
        val elapsed = fix.clock.elapsedMillis - last.clock.elapsedMillis
        if (elapsed <= 0) return refuse("out-of-order")
        if (elapsed > policy.maximumGapMillis) return DriverGpsQuality(scope, policy, fix, fix, total, count, rejected, true)
        val step = separation(last, fix)
        if (step * 1000 / elapsed > policy.maximumSpeedMetresPerSecond) return refuse("impossible-jump")
        val displacement = separation(requireNotNull(anchor), fix)
        if (displacement <= policy.driftBandMetres) return DriverGpsQuality(scope, policy, fix, anchor, total, count, rejected, gap)
        val next = total + displacement
        require(next.isFinite() && next <= DRIVER_MAX_AMOUNT) { "driver-gps-distance-out-of-bounds" }
        return DriverGpsQuality(scope, policy, fix, fix, next, count, rejected, gap)
    }
    override fun toString() = "DriverGpsQuality(aggregate=${estimate()})"
}
