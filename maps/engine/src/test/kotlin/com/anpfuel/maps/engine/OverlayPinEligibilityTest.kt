package com.anpfuel.maps.engine

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
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
        // KML contains only the reviewed pin, labelled by station text, not UUID.
        val kml = buildOverlayKml(preparation.clusters)
        assertTrue(kml.contains("<name>Reviewed</name>"))
        assertTrue(!kml.contains("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"))
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

    @Test fun `every pin tier becomes a pin with its own label and approximate tiers are styled apart`() {
        val reviewed = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
        val osm = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
        val anp = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
        val address = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
        val centroid = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
        val preparation = prepareOverlay(
            listOf(
                marker(reviewed, MapStationLocationQuality.REVIEWED, "Reviewed").copy(latitude = -25.40),
                marker(osm, MapStationLocationQuality.OSM_MAPPED, "Osm").copy(latitude = -25.41),
                marker(anp, MapStationLocationQuality.ANP_REPORTED, "Anp").copy(latitude = -25.42),
                marker(address, MapStationLocationQuality.ADDRESS_POINT, "Address").copy(latitude = -25.43),
                marker(centroid, MapStationLocationQuality.CITY_CENTROID, "Centroid").copy(latitude = -25.44),
            ),
            radiusMeters = 60.0,
            generation = 7,
        )
        assertEquals(
            setOf(reviewed, osm, anp, address),
            preparation.clusters.flatMap { it.markers }.map { it.uuid }.toSet(),
        )
        assertEquals(listOf(centroid), preparation.unpinned.map { it.uuid })
        val kml = buildOverlayKml(preparation.clusters)
        assertEquals(2, Regex("#placemark-yellow").findAll(kml).count())
        assertEquals(2, Regex("#placemark-orange").findAll(kml).count())
        assertTrue(kml.contains("<description>Anp - Fuel - Location reported by ANP</description>"))
        assertTrue(kml.contains("<description>Address - Fuel - Approximate location from address (IBGE)</description>"))
        assertTrue(kml.contains("<description>Osm - Fuel - Mapped by the OpenStreetMap community</description>"))
        val items = overlayListItems(preparation.clusters, unpinned = preparation.unpinned)
        assertEquals("Mapped by the OpenStreetMap community", items.single { it.uuid == osm }.locationLabel)
        assertEquals("Location reported by ANP", items.single { it.uuid == anp }.locationLabel)
        assertEquals("Approximate location from address (IBGE)", items.single { it.uuid == address }.locationLabel)
    }

    @Test fun `pin tiers can route and duplicates keep the best tier location`() {
        assertTrue(marker(ANY, MapStationLocationQuality.ADDRESS_POINT).canRouteByCoordinates)
        assertTrue(marker(ANY, MapStationLocationQuality.ANP_REPORTED).canRouteByCoordinates)
        assertTrue(marker(ANY, MapStationLocationQuality.OSM_MAPPED).canRouteByCoordinates)
        assertFalse(marker(ANY, MapStationLocationQuality.CITY_CENTROID).canRouteByCoordinates)
        assertFalse(marker(ANY, MapStationLocationQuality.UNKNOWN).canRouteByCoordinates)
        val merged = prepareOverlay(
            listOf(
                marker(ANY, MapStationLocationQuality.ADDRESS_POINT).copy(latitude = -25.0),
                marker(ANY, MapStationLocationQuality.OSM_MAPPED).copy(latitude = -25.1),
                marker(ANY, MapStationLocationQuality.ANP_REPORTED).copy(latitude = -25.2),
            ),
        ).clusters.single().markers.single()
        assertEquals(MapStationLocationQuality.OSM_MAPPED, merged.locationQuality)
        assertEquals(-25.1, merged.latitude)
    }

    private companion object {
        const val ANY = "ffffffff-ffff-4fff-8fff-ffffffffffff"
    }
}
