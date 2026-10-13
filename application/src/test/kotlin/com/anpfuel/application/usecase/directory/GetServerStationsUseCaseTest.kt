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
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.async
import kotlinx.coroutines.launch
import org.junit.jupiter.api.Assertions.assertInstanceOf
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(kotlinx.coroutines.ExperimentalCoroutinesApi::class)
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

    @Test
    fun `cancellation propagates instead of replaying cache`() = runTest {
        every { flag.isEnabled() } returns true
        cache.savePage(ServerStationPage(listOf(station()), null))
        coEvery { gateway.list(20, null) } throws kotlinx.coroutines.CancellationException("screen stopped")

        org.junit.jupiter.api.assertThrows<kotlinx.coroutines.CancellationException> { listUseCase() }
        org.junit.jupiter.api.Assertions.assertEquals(1, cache.loadPage()?.items?.size)
    }

    @Test
    fun `offline read uses only cache and disabled flag does not replay it`() = runTest {
        every { flag.isEnabled() } returns true
        cache.savePage(ServerStationPage(listOf(station()), null))
        assertInstanceOf(ServerStationsOutcome.StaleCache::class.java, listUseCase.cached())
        coVerify(exactly = 0) { gateway.list(any(), any()) }
        every { flag.isEnabled() } returns false
        assertInstanceOf(ServerStationsOutcome.Disabled::class.java, listUseCase.cached())
    }

    @Test
    fun `foreground and background refresh cannot overlap`() = runTest {
        every { flag.isEnabled() } returns true
        val release = kotlinx.coroutines.CompletableDeferred<Unit>()
        var requests = 0
        coEvery { gateway.list(20, null) } coAnswers {
            requests++
            if (requests == 1) release.await()
            ServerStationPage(listOf(station()), null)
        }
        val first = async { listUseCase() }
        runCurrent()
        val second = async { listUseCase() }
        runCurrent()
        org.junit.jupiter.api.Assertions.assertEquals(1, requests)
        release.complete(Unit)
        first.await()
        second.await()
        org.junit.jupiter.api.Assertions.assertEquals(2, requests)
    }

    @Test
    fun `cancelled noncooperative gateway cannot save an old page`() = runTest {
        every { flag.isEnabled() } returns true
        val release = kotlinx.coroutines.CompletableDeferred<Unit>()
        coEvery { gateway.list(20, null) } coAnswers {
            kotlinx.coroutines.withContext(kotlinx.coroutines.NonCancellable) { release.await() }
            ServerStationPage(listOf(station()), null)
        }
        val request = launch { listUseCase() }
        runCurrent()
        request.cancel()
        release.complete(Unit)
        request.join()
        org.junit.jupiter.api.Assertions.assertNull(cache.loadPage())
    }

    @Test
    fun `selected city never queries national page or replays another city cache`() = runTest {
        every { flag.isEnabled() } returns true
        var city: String? = "3550308"
        val scoped = GetServerStationsUseCase(flag, gateway, cache,
            com.anpfuel.application.port.StationDirectoryScopeProvider { city })
        coEvery { gateway.listInMunicipality("3550308", 100, null) } returns ServerStationPage(listOf(station()), null)
        assertInstanceOf(ServerStationsOutcome.Fresh::class.java, scoped(100))
        city = "5107925"
        coEvery { gateway.listInMunicipality("5107925", 100, null) } throws IOException("offline")
        assertInstanceOf(ServerStationsOutcome.Unavailable::class.java, scoped(100))
        assertInstanceOf(ServerStationsOutcome.Unavailable::class.java, scoped.cached())
        city = null
        assertInstanceOf(ServerStationsOutcome.Unavailable::class.java, scoped(100))
        coVerify(exactly = 0) { gateway.list(any(), any()) }
    }

    @Test
    fun `foreign municipality returned by server is rejected before caching`() = runTest {
        every { flag.isEnabled() } returns true
        val scoped = GetServerStationsUseCase(flag, gateway, cache,
            com.anpfuel.application.port.StationDirectoryScopeProvider { "5107925" })
        coEvery { gateway.listInMunicipality("5107925", 100, null) } returns ServerStationPage(listOf(station()), null)
        assertInstanceOf(ServerStationsOutcome.Unavailable::class.java, scoped(100))
        org.junit.jupiter.api.Assertions.assertNull(cache.loadScopedPage("5107925"))
    }

    private class FakeServerStationCache : ServerStationCache {
        private var page: ServerStationPage? = null
        private val scopedPages = mutableMapOf<String, ServerStationPage>()
        override suspend fun saveScopedPage(municipalityCode: String, page: ServerStationPage) {
            scopedPages[municipalityCode] = page
        }
        override suspend fun loadScopedPage(municipalityCode: String): ServerStationPage? = scopedPages[municipalityCode]
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
            scopedPages.clear()
            details.clear()
        }
    }
}
