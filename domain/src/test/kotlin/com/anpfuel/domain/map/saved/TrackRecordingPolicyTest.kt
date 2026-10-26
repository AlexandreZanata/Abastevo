package com.anpfuel.domain.map.saved

import com.anpfuel.domain.routing.NavigationFix
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class TrackRecordingPolicyTest {
    private val now = 1_000_000_000_000L
    private fun fix(lat: Double, lon: Double = -49.27, accuracy: Float = 5f, age: Long = 0, alt: Double = 900.12) =
        NavigationFix(lat, lon, accuracy, 1_790_000_000_000, now - age, altitudeMeters = alt)

    @Test fun `distance matches a hand calculated great circle value`() {
        // One degree of latitude on the 6,371,008.8 m mean sphere is 111,194.93 m.
        assertEquals(111_194.93, TrackRecordingPolicy.metres(GeoPoint.parse("0", "0"), GeoPoint.parse("1", "0")), 0.5)
        assertEquals(0.0, TrackRecordingPolicy.metres(GeoPoint(5, 5), GeoPoint(5, 5)), 0.0)
        // 0.0001 degree of latitude = 11.119 m.
        assertEquals(11.119, TrackRecordingPolicy.metres(GeoPoint.parse("-25.0000", "-49.27"), GeoPoint.parse("-25.0001", "-49.27")), 0.01)
    }

    @Test fun `first fix is kept and later ones need at least ten metres`() {
        val first = TrackRecordingPolicy.accept(fix(-25.0), now, null)!!
        assertEquals(GeoPoint.parse("-25.0", "-49.27"), first.point)
        assertEquals(90_012, first.elevationCm)
        assertNull(TrackRecordingPolicy.accept(fix(-25.00005), now, first)) // 5.6 m
        assertNotNull(TrackRecordingPolicy.accept(fix(-25.0001), now, first)) // 11.1 m
        assertNull(TrackRecordingPolicy.accept(fix(-25.0), now, first))
    }

    @Test fun `poor stale or non finite fixes never become points`() {
        assertNull(TrackRecordingPolicy.accept(fix(-25.0, accuracy = 51f), now, null))
        assertNull(TrackRecordingPolicy.accept(fix(-25.0, accuracy = 0f), now, null))
        assertNull(TrackRecordingPolicy.accept(fix(-25.0, age = 15_000_000_001L), now, null))
        assertNull(TrackRecordingPolicy.accept(fix(Double.NaN), now, null))
        assertNull(TrackRecordingPolicy.accept(fix(95.0), now, null))
        assertNotNull(TrackRecordingPolicy.accept(fix(-25.0, accuracy = 50f, age = 15_000_000_000L), now, null))
    }
}
