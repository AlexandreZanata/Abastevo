package com.anpfuel.domain.discovery

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
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
}
