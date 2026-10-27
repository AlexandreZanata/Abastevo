package com.anpfuel.data.remote

import com.anpfuel.domain.discovery.NearbyServerStation
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.ServerStationPage
import com.anpfuel.domain.discovery.StationLocationQuality
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class DirectoryStationJsonCodecTest {

    @Test
    fun `malformed list envelope cannot clear a good cache`() {
        listOf("{}", "{\"items\":null}", "{\"items\":{}}").forEach { payload ->
            assertThrows(Exception::class.java) { DirectoryStationJsonCodec.decodeList(payload) }
        }
    }

    @Test
    fun `decodes station list with reviewed and unknown locations`() {
        val payload = """
        {
          "items": [
            {
              "station_id": "d6c74c23-63db-4c24-a2e5-408cb23bad26",
              "display_name": "Posto Central",
              "address": {"street":"Synthetic Street","number":"12"},
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
        assertEquals("Synthetic Street", page.items[0].address["street"])
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
    fun `decodes every widened pin tier and tolerates an unrecognised future tier`() {
        fun item(id: String, quality: String, coordinates: String) = """
            {"station_id": "$id", "display_name": "Posto $quality", "location_quality": "$quality",
             "coordinates": $coordinates}
        """
        val pin = """{"lat": -23.55, "lon": -46.63}"""
        val payload = """
        {
          "items": [
            ${item("11111111-1111-4111-8111-111111111111", "osm-mapped", pin)},
            ${item("22222222-2222-4222-8222-222222222222", "anp-reported", pin)},
            ${item("33333333-3333-4333-8333-333333333333", "address-point", pin)},
            ${item("44444444-4444-4444-8444-444444444444", "drone-surveyed", pin)},
            ${item("55555555-5555-4555-8555-555555555555", "city-centroid", "null")}
          ],
          "next_cursor": null
        }
        """.trimIndent()

        val items = DirectoryStationJsonCodec.decodeList(payload).items

        assertEquals(5, items.size)
        assertEquals(
            listOf(
                StationLocationQuality.OSM_MAPPED,
                StationLocationQuality.ANP_REPORTED,
                StationLocationQuality.ADDRESS_POINT,
                StationLocationQuality.UNKNOWN,
                StationLocationQuality.CITY_CENTROID,
            ),
            items.map { it.locationQuality },
        )
        assertEquals(listOf(true, true, true, false, false), items.map { it.hasPin })
        assertEquals(-23.55, items[2].latitude)
        assertNull(items[4].latitude)
    }

    @Test
    fun `malformed uuid is rejected`() {
        val payload = """{"station_id": "bad", "display_name": "X", "location_quality": "unknown"}"""
        assertThrows(Exception::class.java) {
            DirectoryStationJsonCodec.decodeStation(payload)
        }
    }

    @Test
    fun `oversized pages cannot expand in memory`() {
        val item = """{"station_id": "d6c74c23-63db-4c24-a2e5-408cb23bad26","display_name": "X","location_quality": "unknown"}"""
        val listPayload = """{"items": [${(1..101).joinToString(",") { item }}]}"""
        assertThrows(Exception::class.java) {
            DirectoryStationJsonCodec.decodeList(listPayload)
        }
        val nearbyItem = """{"station_id": "d6c74c23-63db-4c24-a2e5-408cb23bad26","display_name": "X","location_quality": "unknown","distance_m": 1.0}"""
        val nearbyPayload = """{"items": [${(1..101).joinToString(",") { nearbyItem }}]}"""
        assertThrows(Exception::class.java) {
            DirectoryStationJsonCodec.decodeNearby(nearbyPayload)
        }
    }

    @Test
    fun `oversized cursor is rejected`() {
        val item = """{"station_id": "d6c74c23-63db-4c24-a2e5-408cb23bad26","display_name": "X","location_quality": "unknown"}"""
        val payload = """{"items": [$item],"next_cursor": "${"c".repeat(5000)}"}"""
        assertThrows(Exception::class.java) {
            DirectoryStationJsonCodec.decodeList(payload)
        }
    }
}
