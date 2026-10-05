package com.anpfuel.data.remote

import com.anpfuel.domain.discovery.NearbyServerStation
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.ServerStationPage
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class DirectoryStationJsonCodecTest {

    @Test
    fun `decodes station list with reviewed and unknown locations`() {
        val payload = """
        {
          "items": [
            {
              "station_id": "d6c74c23-63db-4c24-a2e5-408cb23bad26",
              "display_name": "Posto Central",
              "cnpj_normalized": "04218406000104",
              "municipality_code": "3550308",
              "state": "SP",
              "location_quality": "reviewed",
              "coordinates": {"lat": -23.55, "lon": -46.63},
              "current_revision_id": "aac027be-d331-40e4-8d63-52ec3b8d2f41"
            },
            {
              "station_id": "e7d85d34-74ec-5d35-b3f6-519dc34ce370",
              "display_name": "[P34-TEST] Posto Gama",
              "cnpj_normalized": "12ABC34501DE35",
              "municipality_code": "3550308",
              "state": "SP",
              "location_quality": "unknown",
              "coordinates": null,
              "current_revision_id": null
            }
          ],
          "next_cursor": null,
          "generated_at": "2026-09-28T00:00:00Z"
        }
        """.trimIndent()

        val page: ServerStationPage = DirectoryStationJsonCodec.decodeList(payload)

        assertEquals(2, page.items.size)
        assertNull(page.nextCursor)
        assertEquals("d6c74c23-63db-4c24-a2e5-408cb23bad26", page.items[0].stationId)
        assertEquals(-23.55, page.items[0].latitude)
        assertNull(page.items[1].latitude)
        assertEquals("12ABC34501DE35", page.items[1].cnpjNormalized)
    }

    @Test
    fun `decodes nearby without echoing request point`() {
        val payload = """
        {
          "items": [
            {
              "station_id": "d6c74c23-63db-4c24-a2e5-408cb23bad26",
              "display_name": "Posto Central",
              "location_quality": "reviewed",
              "coordinates": {"lat": -23.55, "lon": -46.63},
              "distance_m": 120.5
            }
          ],
          "generated_at": "2026-09-28T00:00:00Z"
        }
        """.trimIndent()

        val items: List<NearbyServerStation> = DirectoryStationJsonCodec.decodeNearby(payload)

        assertEquals(1, items.size)
        assertEquals(120.5, items[0].distanceMeters)
        assertEquals("d6c74c23-63db-4c24-a2e5-408cb23bad26", items[0].station.stationId)
    }

    @Test
    fun `malformed uuid is rejected`() {
        val payload = """{"station_id": "bad", "display_name": "X", "location_quality": "unknown"}"""
        assertThrows(Exception::class.java) {
            DirectoryStationJsonCodec.decodeStation(payload)
        }
    }
}
