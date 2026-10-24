package com.anpfuel.domain.charging

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class ChargingSnapshotFreshnessTest {
    @Test
    fun `missing or future timestamps stay unknown`() {
        assertEquals(ChargingSnapshotAge.UNKNOWN, ChargingSnapshotFreshness.classify(1_000L, null))
        assertEquals(ChargingSnapshotAge.UNKNOWN, ChargingSnapshotFreshness.classify(1_000L, 0L))
        assertEquals(ChargingSnapshotAge.UNKNOWN, ChargingSnapshotFreshness.classify(1_000L, 2_000L))
    }

    @Test
    fun `fresh boundary and stale expiry are exact`() {
        val day = ChargingSnapshotFreshness.STALE_AFTER_MILLIS
        assertEquals(ChargingSnapshotAge.FRESH, ChargingSnapshotFreshness.classify(day, 0L + 1L))
        assertEquals(ChargingSnapshotAge.FRESH, ChargingSnapshotFreshness.classify(day - 1L, 0L + 1L))
        assertEquals(ChargingSnapshotAge.STALE, ChargingSnapshotFreshness.classify(day + 2L, 1L))
    }
}
