package com.anpfuel.maps.engine

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class OverlayKmlTest {
    @Test
    fun `late import cannot commit into a newer overlay generation`() {
        assertTrue(isOverlayCategoryGeneration("Abastevo overlay v1:g12:2", 12))
        assertTrue(isOwnedOverlayCategory("Abastevo overlay v1:g12:2"))
        assertFalse(isOverlayCategoryGeneration("Abastevo overlay v1:g1:2", 12))
        assertFalse(isOverlayCategoryGeneration("Abastevo overlay v1:g12:other", 12))
    }

    @Test
    fun `native duplicate name suffixes retain ownership without adopting unrelated categories`() {
        assertTrue(isOwnedOverlayCategory("Abastevo overlay v1:"))
        assertTrue(isOwnedOverlayCategory("Abastevo overlay v1:12"))
        assertFalse(isOwnedOverlayCategory("Abastevo"))
        assertFalse(isOwnedOverlayCategory("Abastevo overlay v2:1"))
        assertFalse(isOwnedOverlayCategory("Abastevo overlay v1:my places"))
    }

    @Test
    fun `kml starts with the xml declaration and keeps canonical markers`() {
        val preparation = prepareOverlay(
            listOf(
                marker(
                    "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
                    title = "Posto & Cia <centro>",
                    quality = MapStationLocationQuality.REVIEWED,
                ),
                marker(
                    "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
                    latitude = -25.43,
                    longitude = -49.28,
                    title = "Recarga",
                    fuel = false,
                    charging = true,
                    quality = MapStationLocationQuality.REVIEWED,
                ),
                marker(
                    "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
                    latitude = -25.44,
                    longitude = -49.29,
                    title = "Hibrido",
                    fuel = true,
                    charging = true,
                    quality = MapStationLocationQuality.REVIEWED,
                ),
            ),
            radiusMeters = 1.0,
            generation = 42,
        )
        val kml = buildOverlayKml(preparation.clusters)
        assertTrue(kml.contains("<name>Abastevo overlay v1:</name>"))

        assertTrue(kml.startsWith("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"))
        assertTrue(kml.contains("<name>Posto &amp; Cia &lt;centro&gt;</name>"))
        assertTrue(kml.contains("<description>Posto &amp; Cia &lt;centro&gt; - Fuel - Reviewed location</description>"))
        assertTrue(kml.contains("<styleUrl>#placemark-orange</styleUrl>"))
        assertTrue(kml.contains("<mwm:icon>Gas</mwm:icon>"))
        assertTrue(kml.contains("<styleUrl>#placemark-teal</styleUrl>"))
        assertTrue(kml.contains("<mwm:icon>ChargingStation</mwm:icon>"))
        assertTrue(kml.contains("<name>Hibrido</name>"))
        assertTrue(kml.contains("<description>Hibrido - Fuel and charging - Reviewed location</description>"))
        assertTrue(kml.contains("<styleUrl>#placemark-purple</styleUrl>"))
        assertFalse(kml.contains("Posto & Cia"))
    }

    @Test
    fun `duplicate UUIDs merge and co-located distinct UUIDs stay separate`() {
        val duplicate = prepareOverlay(
            listOf(
                marker("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", quality = MapStationLocationQuality.UNKNOWN),
                marker(
                    "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA",
                    fuel = false,
                    charging = true,
                    quality = MapStationLocationQuality.REVIEWED,
                ),
            ),
            radiusMeters = 60.0,
            generation = 7,
        )

        assertEquals(1, duplicate.duplicates)
        assertEquals(0, duplicate.rejected)
        assertEquals(1, duplicate.clusters.single().markers.size)
        val merged = duplicate.clusters.single().markers.single()
        assertEquals("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", merged.uuid)
        assertTrue(merged.fuel)
        assertTrue(merged.charging)
        assertEquals(MapStationLocationQuality.REVIEWED, merged.locationQuality)

        val coLocated = prepareOverlay(
            listOf(
                marker(
                    "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
                    title = "Posto",
                    quality = MapStationLocationQuality.REVIEWED,
                ),
                marker(
                    "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
                    title = "Recarga",
                    fuel = false,
                    charging = true,
                    quality = MapStationLocationQuality.REVIEWED,
                ),
            ),
            radiusMeters = 60.0,
            generation = 8,
        )
        assertEquals(0, coLocated.duplicates)
        assertEquals(1, coLocated.clusters.size)
        assertEquals(2, coLocated.clusters.single().markers.size)
        assertEquals(
            setOf("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
            coLocated.clusters.single().markers.map { it.uuid }.toSet(),
        )
        assertEquals(2, overlayListItems(coLocated.clusters, unpinned = coLocated.unpinned).size)
    }

    @Test
    fun `nearby markers cluster while far markers stay single`() {
        val preparation = prepareOverlay(
            listOf(
                marker(
                    "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
                    latitude = -25.4200,
                    longitude = -49.2700,
                    quality = MapStationLocationQuality.REVIEWED,
                ),
                marker(
                    "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
                    latitude = -25.4201,
                    longitude = -49.2701,
                    quality = MapStationLocationQuality.REVIEWED,
                ),
                marker(
                    "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
                    latitude = -25.4202,
                    longitude = -49.2702,
                    quality = MapStationLocationQuality.REVIEWED,
                ),
                marker(
                    "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
                    latitude = -25.4300,
                    longitude = -49.2800,
                    quality = MapStationLocationQuality.REVIEWED,
                ),
            ),
            radiusMeters = 60.0,
            generation = 9,
        )

        assertEquals(2, preparation.clusters.size)
        assertEquals(3, preparation.clusters[0].markers.size)
        assertTrue(preparation.clusters[0].isCluster)
        assertEquals("c9-0", preparation.clusters[0].id)
        assertFalse(preparation.clusters[1].isCluster)
        assertEquals(4, overlayListItems(preparation.clusters, unpinned = preparation.unpinned).size)
    }

    @Test
    fun `invalid markers are rejected`() {
        val preparation = prepareOverlay(
            listOf(
                marker("not-a-uuid"),
                marker("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", latitude = Double.NaN),
                marker("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", fuel = false, charging = false),
            ),
        )

        assertEquals(3, preparation.rejected)
        assertTrue(preparation.clusters.isEmpty())
    }

    @Test
    fun `cluster kml uses the cluster title and information icon`() {
        val preparation = prepareOverlay(
            listOf(
                marker(
                    "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
                    quality = MapStationLocationQuality.REVIEWED,
                ),
                marker(
                    "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
                    latitude = -25.4201,
                    longitude = -49.2701,
                    quality = MapStationLocationQuality.REVIEWED,
                ),
            ),
            radiusMeters = 60.0,
            generation = 10,
        )
        val kml = buildOverlayKml(preparation.clusters)

        assertTrue(kml.contains("<name>2</name>"))
        assertTrue(kml.contains("<description>2 stations</description>"))
        assertTrue(kml.contains("<styleUrl>#placemark-bluegray</styleUrl>"))
        assertTrue(kml.contains("<mwm:icon>Information</mwm:icon>"))
    }

    @Test
    fun `accessible list items carry type location and title`() {
        val preparation = prepareOverlay(
            listOf(
                marker(
                    "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
                    title = "Posto Central",
                    fuel = true,
                    charging = true,
                    quality = MapStationLocationQuality.REVIEWED,
                ),
                marker(
                    "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
                    title = "Recarga Aproximada",
                    fuel = false,
                    charging = true,
                    quality = MapStationLocationQuality.CITY_CENTROID,
                ),
                marker(
                    "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
                    title = "Posto Sem Local",
                    quality = MapStationLocationQuality.UNKNOWN,
                ),
            ),
            radiusMeters = 1.0,
            generation = 11,
        )
        val items = overlayListItems(preparation.clusters, unpinned = preparation.unpinned)

        assertEquals(3, items.size)
        val central = items.single { it.uuid == "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" }
        assertEquals(MapStationType.FUEL_AND_CHARGING, central.type)
        assertEquals("Fuel and charging", central.typeLabel)
        assertEquals("Reviewed location", central.locationLabel)
        assertEquals("Posto Central, Fuel and charging, Reviewed location", central.contentDescription)
        val approximate = items.single { it.uuid == "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" }
        assertEquals(MapStationType.CHARGING, approximate.type)
        assertEquals("Approximate city location", approximate.locationLabel)
        val unknown = items.single { it.uuid == "cccccccc-cccc-4ccc-8ccc-cccccccccccc" }
        assertEquals(MapStationType.FUEL, unknown.type)
        assertEquals("Location unknown", unknown.locationLabel)
    }

    @Test
    fun `approximate fuel tiers keep the yellow gas pin distinct from reviewed orange`() {
        val preparation = prepareOverlay(
            listOf(
                marker(
                    "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
                    quality = MapStationLocationQuality.ANP_REPORTED,
                ),
            ),
            radiusMeters = 1.0,
            generation = 20,
        )
        val kml = buildOverlayKml(preparation.clusters)
        assertTrue(kml.contains("<styleUrl>#placemark-yellow</styleUrl>"))
        assertTrue(kml.contains("<mwm:icon>Gas</mwm:icon>"))
        assertFalse(kml.contains("#placemark-orange"))
    }

    @Test
    fun `long station names truncate on the map label but keep the full description`() {
        val longTitle = "Posto Rodovia BR-163 Km 742 Margem Direita Sentido Norte Sorriso"
        val preparation = prepareOverlay(
            listOf(
                marker(
                    "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
                    title = longTitle,
                    quality = MapStationLocationQuality.REVIEWED,
                ),
            ),
            radiusMeters = 1.0,
            generation = 21,
        )
        val kml = buildOverlayKml(preparation.clusters)
        assertFalse(kml.contains("<name>$longTitle</name>"), "map label must truncate long names")
        assertTrue(kml.contains("…</name>"), "truncated map label keeps an ellipsis")
        assertTrue(kml.contains(longTitle), "description keeps the full station name")
        val short = buildOverlayKml(
            prepareOverlay(
                listOf(
                    marker(
                        "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
                        title = "Posto Centro",
                        quality = MapStationLocationQuality.REVIEWED,
                    ),
                ),
                radiusMeters = 1.0,
                generation = 22,
            ).clusters,
        )
        assertTrue(short.contains("<name>Posto Centro</name>"), "short names stay untouched")
    }

    @Test
    fun `selection maps exact rendered points and never technical labels`() {
        val first = marker("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", latitude = -25.41, longitude = -49.26)
        val second = marker("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", latitude = -25.42, longitude = -49.27)
        val single = MapOverlayCluster("c1-0", -25.41, -49.26, listOf(first))
        val cluster = MapOverlayCluster("c1-1", -25.42, -49.27, listOf(second, marker(
            "cccccccc-cccc-4ccc-8ccc-cccccccccccc", latitude = -25.4201, longitude = -49.2701,
        )))
        val clusters = listOf(single, cluster)
        val markers = clusters.flatMap { it.markers }

        val station = overlaySelectionFor(-25.41, -49.26, markers, clusters)
        assertTrue(station is MapOverlaySelection.Station)
        assertEquals("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", (station as MapOverlaySelection.Station).uuid)

        val clusterSelection = overlaySelectionFor(-25.42, -49.27, markers, clusters)
        assertTrue(clusterSelection is MapOverlaySelection.Cluster)
        assertEquals(2, (clusterSelection as MapOverlaySelection.Cluster).members.size)

        assertNull(overlaySelectionFor(-25.5, -49.5, markers, clusters))
    }

    private fun marker(
        uuid: String,
        latitude: Double = -25.42,
        longitude: Double = -49.27,
        title: String = "Posto",
        fuel: Boolean = true,
        charging: Boolean = false,
        quality: MapStationLocationQuality = MapStationLocationQuality.UNKNOWN,
    ) = MapOverlayMarker(
        uuid = uuid,
        latitude = latitude,
        longitude = longitude,
        title = title,
        fuel = fuel,
        charging = charging,
        locationQuality = quality,
    )
}
