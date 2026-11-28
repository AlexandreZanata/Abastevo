package com.anpfuel.application.usecase.location

import com.anpfuel.application.usecase.directory.GetNearbyServerStationsUseCase
import com.anpfuel.application.usecase.directory.NearbyServerStationsOutcome
import com.anpfuel.domain.community.FeedCity
import com.anpfuel.domain.discovery.DeviceProximityRule
import com.anpfuel.domain.discovery.NearbyServerStation
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DeviceLocation
import com.anpfuel.domain.valueobject.MunicipalityCatalogEntry
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import java.io.IOException
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

/** B-BR-LOC-01..03: device position to nearby stations, their city and the followed preferred city. */
class FollowDeviceCityUseCaseTest {

    private val nearby = mockk<GetNearbyServerStationsUseCase>()
    private val catalog = mockk<MunicipalityCatalogRepository>()
    private val locations = mockk<SelectLocationUseCase>()
    private val useCase = FollowDeviceCityUseCase(nearby, catalog, locations)
    private val fix = DeviceLocation.of(-12.55, -55.72)

    private val sorriso = FeedCity("5107925", BrazilianState.MATO_GROSSO, "SORRISO")
    private val sinop = FeedCity("5107909", BrazilianState.MATO_GROSSO, "SINOP")

    private fun station(id: Int, distance: Double, code: String = sorriso.code, state: String = "MT") =
        NearbyServerStation(
            station = ServerStation.create(
                stationId = "00000000-0000-4000-8000-%012d".format(id),
                displayName = "Posto $id",
                locationQuality = StationLocationQuality.REVIEWED,
                latitude = -12.55,
                longitude = -55.72,
                cnpjNormalized = null,
                municipalityCode = code,
                state = state,
                currentRevisionId = null,
            ),
            distanceMeters = distance,
        )

    private fun givenCatalog() {
        coEvery { catalog.getCatalogMunicipalities(BrazilianState.MATO_GROSSO) } returns listOf(
            MunicipalityCatalogEntry(BrazilianState.MATO_GROSSO, "SINOP", sinop.code),
            MunicipalityCatalogEntry(BrazilianState.MATO_GROSSO, "SORRISO", sorriso.code),
        )
        coEvery { catalog.findCatalogEntry(BrazilianState.MATO_GROSSO, "SINOP") } returns
            MunicipalityCatalogEntry(BrazilianState.MATO_GROSSO, "SINOP", sinop.code)
        coEvery { catalog.findCatalogEntry(BrazilianState.MATO_GROSSO, "SORRISO") } returns
            MunicipalityCatalogEntry(BrazilianState.MATO_GROSSO, "SORRISO", sorriso.code)
    }

    @Test
    fun `surroundings read one bounded nearby page and resolve the nearest station city`() = runTest {
        givenCatalog()
        val near = station(1, 40.0)
        val far = station(2, 3_000.0, code = sinop.code)
        coEvery { nearby(fix.latitude, fix.longitude, any(), any()) } returns NearbyServerStationsOutcome.Fresh(listOf(far, near))

        val surroundings = useCase.surroundings(fix)!!

        assertEquals(listOf(near, far), surroundings.stations)
        assertEquals(sorriso, surroundings.city)
        assertEquals(near, surroundings.preselectable)
        coVerify(exactly = 1) {
            nearby(fix.latitude, fix.longitude, DeviceProximityRule.LOOKUP_RADIUS_METERS, DeviceProximityRule.LOOKUP_LIMIT)
        }
    }

    @Test
    fun `surroundings have no city when the code is not in the local catalog`() = runTest {
        givenCatalog()
        coEvery { nearby(any(), any(), any(), any()) } returns
            NearbyServerStationsOutcome.Fresh(listOf(station(1, 40.0, code = "5100102")))

        val surroundings = useCase.surroundings(fix)!!

        assertNull(surroundings.city)
        assertEquals(1, surroundings.stations.size)
    }

    @Test
    fun `surroundings are unavailable when the directory read is off or fails`() = runTest {
        coEvery { nearby(any(), any(), any(), any()) } returns NearbyServerStationsOutcome.Disabled
        assertNull(useCase.surroundings(fix))

        coEvery { nearby(any(), any(), any(), any()) } returns NearbyServerStationsOutcome.Unavailable(IOException("offline"))
        assertNull(useCase.surroundings(fix))
    }

    @Test
    fun `follow switches the preferred city and keeps the previous one for undo`() = runTest {
        givenCatalog()
        coEvery { locations.getPreferredLocation() } returns PreferredLocation(BrazilianState.MATO_GROSSO, "SINOP")
        coEvery { locations.selectLocation(BrazilianState.MATO_GROSSO, "SORRISO") } returns mockk()

        val outcome = useCase.follow(sorriso)

        assertEquals(
            CityFollowOutcome.Switched(PreferredLocation(BrazilianState.MATO_GROSSO, "SINOP"), sorriso),
            outcome,
        )
        coVerify(exactly = 1) { locations.selectLocation(BrazilianState.MATO_GROSSO, "SORRISO") }
    }

    @Test
    fun `follow leaves the preferred city alone when the device is already there`() = runTest {
        givenCatalog()
        coEvery { locations.getPreferredLocation() } returns PreferredLocation(BrazilianState.MATO_GROSSO, "Sorriso")
        coEvery { catalog.findCatalogEntry(BrazilianState.MATO_GROSSO, "Sorriso") } returns
            MunicipalityCatalogEntry(BrazilianState.MATO_GROSSO, "SORRISO", sorriso.code)

        assertEquals(CityFollowOutcome.AlreadyThere, useCase.follow(sorriso))
        coVerify(exactly = 0) { locations.selectLocation(any(), any()) }
    }

    @Test
    fun `follow selects a first city when none was chosen yet`() = runTest {
        givenCatalog()
        coEvery { locations.getPreferredLocation() } returns null
        coEvery { locations.selectLocation(BrazilianState.MATO_GROSSO, "SORRISO") } returns mockk()

        assertEquals(CityFollowOutcome.Switched(null, sorriso), useCase.follow(sorriso))
    }

    @Test
    fun `follow reports unavailable when the selection is refused`() = runTest {
        givenCatalog()
        coEvery { locations.getPreferredLocation() } returns PreferredLocation(BrazilianState.MATO_GROSSO, "SINOP")
        coEvery { locations.selectLocation(any(), any()) } throws com.anpfuel.domain.exception.DomainException("BR-006")

        assertEquals(CityFollowOutcome.Unavailable, useCase.follow(sorriso))
    }

    @Test
    fun `follow never swallows cancellation`() = runTest {
        givenCatalog()
        coEvery { locations.getPreferredLocation() } returns PreferredLocation(BrazilianState.MATO_GROSSO, "SINOP")
        coEvery { locations.selectLocation(any(), any()) } throws CancellationException("left")

        assertThrows<CancellationException> { useCase.follow(sorriso) }
    }

    @Test
    fun `restore puts the previous city back`() = runTest {
        val previous = PreferredLocation(BrazilianState.MATO_GROSSO, "SINOP")
        coEvery { locations.selectLocation(BrazilianState.MATO_GROSSO, "SINOP") } returns mockk()

        assertTrue(useCase.restore(previous))
        coVerify(exactly = 1) { locations.selectLocation(BrazilianState.MATO_GROSSO, "SINOP") }

        coEvery { locations.selectLocation(any(), any()) } throws IllegalStateException("db")
        assertFalse(useCase.restore(previous))
    }
}
