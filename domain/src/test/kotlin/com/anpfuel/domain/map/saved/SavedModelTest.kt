package com.anpfuel.domain.map.saved

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class SavedModelTest {
    private fun code(block: () -> Unit): String = try { block(); "ok" } catch (e: SavedValidationException) { e.code }
    private val id1 = "11111111-1111-4111-8111-111111111111"
    private val id2 = "22222222-2222-4222-8222-222222222222"

    @Test fun `coordinates are exact micro degrees hand calculated`() {
        assertEquals(GeoPoint(-25_429_500, -49_271_900), GeoPoint.parse("-25.4295", "-49.2719"))
        assertEquals(-25_429_568L, GeoPoint.parse("-25.4295675", "0").latMicro) // half-up away from zero at .5 micro
        assertEquals(90_000_000L, GeoPoint.parse("90", "180").latMicro)
        assertEquals(-180_000_000L, GeoPoint.parse("0", "-180.000000000").lonMicro)
        assertEquals(0L, GeoPoint.parse("0.0000004", "0").latMicro)
        assertEquals(1L, GeoPoint.parse("0.0000005", "0").latMicro)
    }

    @Test fun `malformed or out of range coordinates are refused never clamped`() {
        for (bad in listOf("", "abc", "1e3", "NaN", "Infinity", "-", "+", "1,5", "91", "-90.0000005", "1.0000000000000000001", " 12 3", "0x10")) {
            assertNotEquals("ok", code { GeoPoint.parse(bad, "0") }, bad)
        }
        assertNotEquals("ok", code { GeoPoint.parse("0", "180.0000005") })
        assertNotEquals("ok", code { GeoPoint.fromDouble(Double.NaN, 0.0) })
        assertNotEquals("ok", code { GeoPoint.fromDouble(0.0, Double.POSITIVE_INFINITY) })
        assertNotEquals("ok", code { GeoPoint.fromDouble(1e300, 0.0) })
        assertEquals(GeoPoint(-25_429_500, -49_271_900), GeoPoint.fromDouble(-25.4295, -49.2719))
    }

    private fun place(kind: SavedPlaceKind = SavedPlaceKind.PLACE, station: String? = null, name: String = "Posto") =
        SavedPlace(id1, kind, name, GeoPoint(1, 1), station, SavedSourceKind.MANUAL, null, null, 0)

    @Test fun `place rules require station id only for stations and bound text`() {
        assertEquals("ok", code { place() })
        assertEquals("ok", code { place(SavedPlaceKind.STATION, id2) })
        assertEquals("station-id-required", code { place(SavedPlaceKind.STATION, null) })
        assertEquals("station-id-forbidden", code { place(SavedPlaceKind.PLACE, id2) })
        assertEquals("place-name-invalid", code { place(name = " ") })
        assertEquals("place-name-invalid", code { place(name = "x".repeat(121)) })
        assertEquals("ok", code { place(name = "x".repeat(120)) })
        assertEquals("place-name-invalid", code { place(name = "bad\u0000name") })
        assertEquals("place-id-invalid", code { SavedPlace("nope", SavedPlaceKind.PLACE, "a", GeoPoint(0, 0), null, SavedSourceKind.MANUAL, null, null, 0) })
    }

    private fun trip(kind: SavedTripKind, n: Int, avoid: Set<String> = emptySet(), distinct: Boolean = true) =
        SavedTrip(id2, kind, "Trip", List(n) { TripPoint(GeoPoint(if (distinct) it.toLong() else 0, 0)) }, avoid, null, null, 0)

    @Test fun `trip rules separate route plans from tracks and enforce bounds`() {
        assertEquals("ok", code { trip(SavedTripKind.ROUTE_PLAN, 2, setOf("TOLL")) })
        assertEquals("route-plan-points-invalid", code { trip(SavedTripKind.ROUTE_PLAN, 1) })
        assertEquals("route-plan-points-invalid", code { trip(SavedTripKind.ROUTE_PLAN, 103) })
        assertEquals("ok", code { trip(SavedTripKind.ROUTE_PLAN, 102) })
        assertEquals("route-plan-points-invalid", code { trip(SavedTripKind.ROUTE_PLAN, 3, distinct = false) })
        assertEquals("avoid-invalid", code { trip(SavedTripKind.ROUTE_PLAN, 2, setOf("SPEEDING")) })
        assertEquals("ok", code { trip(SavedTripKind.RECORDED_TRACK, 10_000, distinct = false) })
        assertEquals("track-points-invalid", code { trip(SavedTripKind.RECORDED_TRACK, 10_001, distinct = false) })
        assertEquals("track-points-invalid", code { trip(SavedTripKind.IMPORTED_TRACK, 0) })
        assertEquals("track-avoid-forbidden", code { trip(SavedTripKind.RECORDED_TRACK, 2, setOf("TOLL")) })
        assertEquals("route-plan-points-invalid", code {
            SavedTrip(id2, SavedTripKind.ROUTE_PLAN, "t", listOf(TripPoint(GeoPoint(0, 0), timeMillis = 5), TripPoint(GeoPoint(1, 1))), createdAtMillis = 0)
        })
    }

    @Test fun `owner scope is mandatory`() {
        assertThrows(SavedValidationException::class.java) { SavedOwner("", "https://x") }
        assertThrows(SavedValidationException::class.java) { SavedOwner("guest", "") }
        assertEquals("guest", SavedOwner.GUEST)
    }
}
