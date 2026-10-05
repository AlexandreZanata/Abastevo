package com.anpfuel.application.usecase.directory

import com.anpfuel.application.port.CommunityReadsFlagProvider
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.ServerStationPage
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.repository.ServerStationCache
import com.anpfuel.domain.repository.ServerStationGateway
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import java.io.IOException
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertInstanceOf
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class GetServerStationsUseCaseTest {

    private val stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"
    private val flag: CommunityReadsFlagProvider = mockk()
    private val gateway: ServerStationGateway = mockk()
    private val cache = FakeServerStationCache()

    private lateinit var listUseCase: GetServerStationsUseCase
    private lateinit var detailUseCase: GetServerStationDetailUseCase

    private fun station() = ServerStation.create(
        stationId = stationId,
        displayName = "Posto Central",
        locationQuality = StationLocationQuality.REVIEWED,
        latitude = -23.55,
        longitude = -46.63,
        cnpjNormalized = "04218406000104",
        municipalityCode = "3550308",
        state = "SP",
        currentRevisionId = null,
    )

    @BeforeEach
    fun setUp() = kotlinx.coroutines.runBlocking {
        cache.clear()
        listUseCase = GetServerStationsUseCase(flag, gateway, cache)
        detailUseCase = GetServerStationDetailUseCase(flag, gateway, cache)
    }

    @Test
    fun `disabled flag never touches network or cache`() = runTest {
        every { flag.isEnabled() } returns false

        val outcome = listUseCase()
        val detailOutcome = detailUseCase(stationId)

        assertInstanceOf(ServerStationsOutcome.Disabled::class.java, outcome)
        assertInstanceOf(ServerStationDetailOutcome.Disabled::class.java, detailOutcome)
        coVerify(exactly = 0) { gateway.list(any(), any()) }
        coVerify(exactly = 0) { gateway.detail(any()) }
    }

    @Test
    fun `list success saves fresh page`() = runTest {
        every { flag.isEnabled() } returns true
        val page = ServerStationPage(listOf(station()), null)
        coEvery { gateway.list(20, null) } returns page

        val outcome = listUseCase()

        assertInstanceOf(ServerStationsOutcome.Fresh::class.java, outcome)
        coVerify { gateway.list(20, null) }
    }

    @Test
    fun `list failure replays last-known cache honestly`() = runTest {
        every { flag.isEnabled() } returns true
        val page = ServerStationPage(listOf(station()), null)
        coEvery { gateway.list(20, null) } returns page
        listUseCase()
        coEvery { gateway.list(20, null) } throws IOException("down")

        val outcome = listUseCase()

        assertInstanceOf(ServerStationsOutcome.StaleCache::class.java, outcome)
    }

    @Test
    fun `list failure without cache is unavailable`() = runTest {
        every { flag.isEnabled() } returns true
        coEvery { gateway.list(20, null) } throws IOException("down")

        val outcome = listUseCase()

        assertInstanceOf(ServerStationsOutcome.Unavailable::class.java, outcome)
    }

    @Test
    fun `blank station id is rejected`() = runTest {
        every { flag.isEnabled() } returns true

        assertThrows(IllegalArgumentException::class.java) {
            kotlinx.coroutines.runBlocking { detailUseCase("  ") }
        }
    }

    @Test
    fun `nearby success returns fresh without touching cache`() = runTest {
        every { flag.isEnabled() } returns true
        val nearby = com.anpfuel.domain.discovery.NearbyServerStation(station(), 120.5)
        val nearbyUseCase = GetNearbyServerStationsUseCase(flag, gateway)
        coEvery { gateway.nearby(-23.55, -46.63, 2000, 20) } returns listOf(nearby)

        val outcome = nearbyUseCase(-23.55, -46.63)

        assertInstanceOf(NearbyServerStationsOutcome.Fresh::class.java, outcome)
    }

    @Test
    fun `nearby failure is unavailable and disabled never calls network`() = runTest {
        every { flag.isEnabled() } returns true
        val nearbyUseCase = GetNearbyServerStationsUseCase(flag, gateway)
        coEvery { gateway.nearby(any(), any(), any(), any()) } throws IOException("down")

        assertInstanceOf(
            NearbyServerStationsOutcome.Unavailable::class.java,
            nearbyUseCase(-23.55, -46.63),
        )

        every { flag.isEnabled() } returns false
        assertInstanceOf(
            NearbyServerStationsOutcome.Disabled::class.java,
            nearbyUseCase(-23.55, -46.63),
        )
    }

    private class FakeServerStationCache : ServerStationCache {
        private var page: ServerStationPage? = null
        private val details = mutableMapOf<String, ServerStation>()

        override suspend fun savePage(page: ServerStationPage) {
            this.page = page
            for (station in page.items) details[station.stationId] = station
        }

        override suspend fun loadPage(): ServerStationPage? = page

        override suspend fun saveDetail(station: ServerStation) {
            details[station.stationId] = station
        }

        override suspend fun loadDetail(stationId: String): ServerStation? =
            details[stationId.lowercase()]

        override suspend fun clear() {
            page = null
            details.clear()
        }
    }
}
