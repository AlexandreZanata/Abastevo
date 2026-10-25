package com.anpfuel.maps.engine

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class OverlayPinEligibilityTest {
    private fun marker(
        uuid: String,
        quality: MapStationLocationQuality,
        title: String = "Station",
    ) = MapOverlayMarker(
        uuid = uuid,
        latitude = -25.42,
        longitude = -49.27,
        title = title,
        fuel = true,
        charging = false,
        locationQuality = quality,
    )

    @Test fun `unknown and city-centroid never become exact pins but stay in list`() {
        val preparation = prepareOverlay(
            listOf(
                marker("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", MapStationLocationQuality.REVIEWED, "Reviewed"),
                marker("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", MapStationLocationQuality.UNKNOWN, "Unknown"),
                marker("cccccccc-cccc-4ccc-8ccc-cccccccccccc", MapStationLocationQuality.CITY_CENTROID, "Centroid"),
            ),
            radiusMeters = 60.0,
            generation = 99,
        )
        // Only REVIEWED becomes a pin.
        assertEquals(1, preparation.clusters.size)
        assertEquals("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", preparation.clusters.single().markers.single().uuid)
        assertEquals(2, preparation.unpinned.size)
        // KML contains only the reviewed pin.
        val kml = buildOverlayKml(preparation.clusters)
        assertTrue(kml.contains("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"))
        assertTrue(!kml.contains("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"))
        assertTrue(!kml.contains("cccccccc-cccc-4ccc-8ccc-cccccccccccc"))
        // Accessible list keeps all three with explicit warnings.
        val items = overlayListItems(preparation.clusters, unpinned = preparation.unpinned)
        assertEquals(3, items.size)
        assertEquals("Reviewed location", items.single { it.uuid.startsWith("aaaaaaaa") }.locationLabel)
        assertEquals("Location unknown", items.single { it.uuid.startsWith("bbbbbbbb") }.locationLabel)
        assertEquals("Approximate city location", items.single { it.uuid.startsWith("cccccccc") }.locationLabel)
    }

    @Test fun `canonical uuid and stable identity survive source changes`() {
        val first = prepareOverlay(
            listOf(marker("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", MapStationLocationQuality.REVIEWED, "Posto")),
            radiusMeters = 60.0,
            generation = 1,
        )
        val second = prepareOverlay(
            listOf(marker("AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA", MapStationLocationQuality.REVIEWED, "Posto Novo")),
            radiusMeters = 60.0,
            generation = 2,
        )
        assertEquals(
            first.clusters.single().markers.single().uuid,
            second.clusters.single().markers.single().uuid,
        )
        assertEquals("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", second.clusters.single().markers.single().uuid)
    }
}
