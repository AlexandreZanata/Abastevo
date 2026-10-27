package com.anpfuel.domain.discovery

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ServerStationTest {

    private val stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"

    @Test
    fun `valid reviewed station preserves identity and coordinates`() {
        val station = ServerStation.create(
            stationId = stationId,
            displayName = "  Posto Central  ",
            locationQuality = StationLocationQuality.REVIEWED,
            latitude = -23.55,
            longitude = -46.63,
            cnpjNormalized = "04218406000104",
            municipalityCode = "3550308",
            state = "SP",
            currentRevisionId = "aac027be-d331-40e4-8d63-52ec3b8d2f41",
        )

        assertEquals(stationId, station.stationId)
        assertEquals("Posto Central", station.displayName)
        assertEquals(-23.55, station.latitude)
        assertEquals(-46.63, station.longitude)
        assertEquals("04218406000104", station.cnpjNormalized)
    }

    @Test
    fun `unknown station without coordinates stays honest`() {
        val station = ServerStation.create(
            stationId = stationId,
            displayName = "[P34-TEST] Posto Gama",
            locationQuality = StationLocationQuality.UNKNOWN,
            latitude = null,
            longitude = null,
            cnpjNormalized = "12ABC34501DE35",
            municipalityCode = "3550308",
            state = "SP",
            currentRevisionId = null,
        )

        assertNull(station.latitude)
        assertNull(station.longitude)
        // Alphanumeric CNPJ preserved verbatim, leading zeros kept.
        assertEquals("12ABC34501DE35", station.cnpjNormalized)
    }

    @Test
    fun `malformed uuid and blank name are rejected`() {
        assertThrows(DomainException::class.java) {
            ServerStation.create(
                stationId = "not-a-uuid",
                displayName = "Posto",
                locationQuality = StationLocationQuality.REVIEWED,
                latitude = -23.55,
                longitude = -46.63,
                cnpjNormalized = null,
                municipalityCode = null,
                state = null,
                currentRevisionId = null,
            )
        }
        assertThrows(DomainException::class.java) {
            ServerStation.create(
                stationId = stationId,
                displayName = "   ",
                locationQuality = StationLocationQuality.UNKNOWN,
                latitude = null,
                longitude = null,
                cnpjNormalized = null,
                municipalityCode = null,
                state = null,
                currentRevisionId = null,
            )
        }
    }

    @Test
    fun `out of range coordinates are rejected`() {
        assertThrows(DomainException::class.java) {
            ServerStation.create(
                stationId = stationId,
                displayName = "Posto",
                locationQuality = StationLocationQuality.REVIEWED,
                latitude = 91.0,
                longitude = -46.63,
                cnpjNormalized = null,
                municipalityCode = null,
                state = null,
                currentRevisionId = null,
            )
        }
    }

    @Test
    fun `partial coordinates are rejected`() {
        assertThrows(DomainException::class.java) {
            ServerStation.create(
                stationId = stationId,
                displayName = "Posto",
                locationQuality = StationLocationQuality.REVIEWED,
                latitude = -23.55,
                longitude = null,
                cnpjNormalized = null,
                municipalityCode = null,
                state = null,
                currentRevisionId = null,
            )
        }
    }

    private fun tiered(
        id: String,
        quality: StationLocationQuality,
        lat: Double? = -23.55,
        lon: Double? = -46.63,
    ) = ServerStation.create(id, "Posto", quality, lat, lon, null, "3550308", "SP", null)

    @Test
    fun `each new pin tier parses from its wire value`() {
        assertEquals(StationLocationQuality.OSM_MAPPED, StationLocationQuality.fromWire("osm-mapped"))
        assertEquals(StationLocationQuality.ANP_REPORTED, StationLocationQuality.fromWire("anp-reported"))
        assertEquals(StationLocationQuality.ADDRESS_POINT, StationLocationQuality.fromWire("address-point"))
    }

    @Test
    fun `unrecognised future wire value degrades to unknown without a pin`() {
        assertEquals(StationLocationQuality.UNKNOWN, StationLocationQuality.fromWire("satellite-guess"))
        assertEquals(StationLocationQuality.UNKNOWN, StationLocationQuality.fromWire(""))
        val station = tiered(stationId, StationLocationQuality.fromWire("satellite-guess"))
        assertFalse(station.hasPin)
    }

    @Test
    fun `pin tiers follow reviewed then osm then anp then address precedence`() {
        val ordered = StationLocationQuality.entries.filter { it.isPin }.sortedByDescending { it.pinRank }
        assertEquals(
            listOf(
                StationLocationQuality.REVIEWED,
                StationLocationQuality.OSM_MAPPED,
                StationLocationQuality.ANP_REPORTED,
                StationLocationQuality.ADDRESS_POINT,
            ),
            ordered,
        )
        assertFalse(StationLocationQuality.UNKNOWN.isPin)
        assertFalse(StationLocationQuality.CITY_CENTROID.isPin)
        assertTrue(StationLocationQuality.ANP_REPORTED.isApproximatePin)
        assertTrue(StationLocationQuality.ADDRESS_POINT.isApproximatePin)
        assertFalse(StationLocationQuality.REVIEWED.isApproximatePin)
        assertFalse(StationLocationQuality.OSM_MAPPED.isApproximatePin)
    }

    @Test
    fun `pin requires a pin tier and coordinates`() {
        assertTrue(tiered(stationId, StationLocationQuality.ADDRESS_POINT).hasPin)
        assertFalse(tiered(stationId, StationLocationQuality.ADDRESS_POINT, null, null).hasPin)
        assertFalse(tiered(stationId, StationLocationQuality.CITY_CENTROID).hasPin)
    }

    @Test
    fun `centering prefers the best pin tier and skips coordinate-less stations`() {
        val address = tiered("11111111-1111-4111-8111-111111111111", StationLocationQuality.ADDRESS_POINT)
        val anp = tiered("22222222-2222-4222-8222-222222222222", StationLocationQuality.ANP_REPORTED)
        val osmNoCoords = tiered("33333333-3333-4333-8333-333333333333", StationLocationQuality.OSM_MAPPED, null, null)
        val centroid = tiered("44444444-4444-4444-8444-444444444444", StationLocationQuality.CITY_CENTROID)
        assertEquals(anp.stationId, listOf(centroid, address, osmNoCoords, anp).bestPinnedStation()?.stationId)
        assertEquals(address.stationId, listOf(centroid, address).bestPinnedStation()?.stationId)
        assertNull(listOf(centroid, osmNoCoords).bestPinnedStation())
    }
}
