package com.anpfuel.domain.charging

/**
 * EE-26 source aging for the public charging snapshot.
 *
 * The snapshot is dated provider metadata, never a live availability
 * claim. Age is computed from the stored pack timestamp only; a missing
 * timestamp stays UNKNOWN rather than being treated as fresh or expired.
 */
enum class ChargingSnapshotAge { UNKNOWN, FRESH, STALE }

object ChargingSnapshotFreshness {
    /** Public pack snapshots are refreshed by an explicit user action. */
    const val STALE_AFTER_MILLIS: Long = 24L * 60L * 60L * 1000L

    fun classify(nowMillis: Long, savedAtMillis: Long?): ChargingSnapshotAge {
        if (savedAtMillis == null || savedAtMillis <= 0L) return ChargingSnapshotAge.UNKNOWN
        if (nowMillis < savedAtMillis) return ChargingSnapshotAge.UNKNOWN
        return if (nowMillis - savedAtMillis >= STALE_AFTER_MILLIS) {
            ChargingSnapshotAge.STALE
        } else {
            ChargingSnapshotAge.FRESH
        }
    }
}
