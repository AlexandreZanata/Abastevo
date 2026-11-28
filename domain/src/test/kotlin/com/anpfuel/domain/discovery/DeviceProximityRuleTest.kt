package com.anpfuel.domain.discovery

import com.anpfuel.domain.valueobject.BrazilianState
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

/** B-BR-LOC-01 and B-BR-LOC-02: what nearby stations say about where the device is. */
class DeviceProximityRuleTest {

    private var nextId = 0

    private fun nearby(
        distanceMeters: Double,
        quality: StationLocationQuality = StationLocationQuality.REVIEWED,
        municipalityCode: String? = "5107925",
        state: String? = "MT",
        withPoint: Boolean = true,
    ): NearbyServerStation {
        nextId += 1
        return NearbyServerStation(
            station = ServerStation.create(
                stationId = "00000000-0000-4000-8000-%012d".format(nextId),
                displayName = "Posto $nextId",
                locationQuality = quality,
                latitude = if (withPoint) -12.5 else null,
                longitude = if (withPoint) -55.7 else null,
                cnpjNormalized = null,
                municipalityCode = municipalityCode,
                state = state,
                currentRevisionId = null,
            ),
            distanceMeters = distanceMeters,
        )
    }

    @Test
    fun `pre-selects the nearest pinned station within the pre-selection radius`() {
        val far = nearby(180.0)
        val near = nearby(35.0)

        assertEquals(near, DeviceProximityRule.preselectable(listOf(far, near)))
    }

    @Test
    fun `pre-selects nothing when the nearest station is beyond the radius`() {
        assertNull(DeviceProximityRule.preselectable(listOf(nearby(DeviceProximityRule.PRESELECT_MAX_METERS + 1))))
    }

    @Test
    fun `accepts a station exactly at the radius`() {
        val edge = nearby(DeviceProximityRule.PRESELECT_MAX_METERS)

        assertEquals(edge, DeviceProximityRule.preselectable(listOf(edge)))
    }

    @Test
    fun `never pre-selects a station without a real pin`() {
        val centroid = nearby(10.0, quality = StationLocationQuality.CITY_CENTROID)
        val unknown = nearby(12.0, quality = StationLocationQuality.UNKNOWN)
        val pinned = nearby(90.0, quality = StationLocationQuality.ADDRESS_POINT)

        assertEquals(pinned, DeviceProximityRule.preselectable(listOf(centroid, unknown, pinned)))
        assertNull(DeviceProximityRule.preselectable(listOf(centroid, unknown)))
    }

    @Test
    fun `ignores negative or non-finite distances`() {
        assertNull(DeviceProximityRule.preselectable(listOf(nearby(-1.0), nearby(Double.NaN))))
    }

    @Test
    fun `municipality comes from the nearest station that names one`() {
        val unnamed = nearby(20.0, municipalityCode = null)
        val nearest = nearby(400.0, municipalityCode = "5107925", state = "MT")
        val other = nearby(900.0, municipalityCode = "5103403", state = "MT")

        assertEquals(
            StationMunicipality("5107925", BrazilianState.MATO_GROSSO),
            DeviceProximityRule.municipalityOf(listOf(other, unnamed, nearest)),
        )
    }

    @Test
    fun `municipality ignores malformed codes and unknown states`() {
        val malformed = nearby(10.0, municipalityCode = "51079")
        val badState = nearby(20.0, state = "XX")
        val valid = nearby(30.0, municipalityCode = "3550308", state = "SP")

        assertEquals(
            StationMunicipality("3550308", BrazilianState.SAO_PAULO),
            DeviceProximityRule.municipalityOf(listOf(malformed, badState, valid)),
        )
    }

    @Test
    fun `municipality is unknown when every station is beyond the city radius`() {
        assertNull(DeviceProximityRule.municipalityOf(listOf(nearby(DeviceProximityRule.CITY_MAX_METERS + 1))))
        assertNull(DeviceProximityRule.municipalityOf(emptyList()))
    }

    @Test
    fun `municipality may come from a station without a precise pin`() {
        val centroid = nearby(500.0, quality = StationLocationQuality.CITY_CENTROID, municipalityCode = "5107925")

        assertEquals(
            StationMunicipality("5107925", BrazilianState.MATO_GROSSO),
            DeviceProximityRule.municipalityOf(listOf(centroid)),
        )
    }
}
