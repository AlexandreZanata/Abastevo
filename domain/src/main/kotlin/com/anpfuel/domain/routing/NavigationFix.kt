package com.anpfuel.domain.routing

/** Ephemeral input: deliberately no generated toString containing GPS. */
class NavigationFix(
    val latitude: Double, val longitude: Double, val accuracyMeters: Float,
    val timeMillis: Long, val elapsedRealtimeNanos: Long,
    val altitudeMeters: Double = 0.0, val speedMetersPerSecond: Float = 0f, val bearingDegrees: Float = 0f,
)
object NavigationFixPolicy {
    const val MAX_AGE_NANOS = 15_000_000_000L
    fun accepts(fix: NavigationFix, nowNanos: Long): Boolean =
        fix.latitude.isFinite() && fix.longitude.isFinite() && fix.latitude in -90.0..90.0 && fix.longitude in -180.0..180.0 &&
            fix.accuracyMeters.isFinite() && fix.accuracyMeters > 0f && fix.accuracyMeters <= 50f &&
            fix.timeMillis > 0 && fix.elapsedRealtimeNanos > 0 && nowNanos >= fix.elapsedRealtimeNanos &&
            nowNanos - fix.elapsedRealtimeNanos <= MAX_AGE_NANOS && fix.altitudeMeters.isFinite() &&
            fix.speedMetersPerSecond.isFinite() && fix.speedMetersPerSecond >= 0 &&
            fix.bearingDegrees.isFinite() && fix.bearingDegrees in 0f..360f
}
data class NavigationProgress(
    val remainingMeters: Double, val seconds: Int, val turnMeters: Double,
    val completionPercent: Double, val direction: String, val nextStreet: String,
    val nativeState: String, val notifications: List<String>,
)
