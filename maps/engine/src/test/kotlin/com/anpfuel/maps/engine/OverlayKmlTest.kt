package com.anpfuel.maps.engine

import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class OverlayKmlTest {
    @Test
    fun `kml starts with the xml declaration and keeps canonical markers`() {
        val kml = buildOverlayKml(
            listOf(
                MapOverlayMarker(
                    uuid = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
                    latitude = -25.42,
                    longitude = -49.27,
                    title = "Posto & Cia <centro>",
                ),
                MapOverlayMarker(
                    uuid = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
                    latitude = -25.43,
                    longitude = -49.26,
                    title = "Recarga",
                    charging = true,
                ),
            ),
        )

        assertTrue(kml.startsWith("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"))
        assertTrue(kml.contains("<name>ABASTEVO:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa</name>"))
        assertTrue(kml.contains("<description>Posto &amp; Cia &lt;centro&gt;</description>"))
        assertTrue(kml.contains("<coordinates>-49.27,-25.42</coordinates>"))
        assertFalse(kml.contains("Posto & Cia"))
    }
}
