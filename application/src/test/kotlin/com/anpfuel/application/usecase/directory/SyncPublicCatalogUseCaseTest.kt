package com.anpfuel.application.usecase.directory

import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.ServerStationPage
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.UserPreferencesRepository
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import java.io.IOException
import kotlinx.coroutines.async
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertInstanceOf
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class SyncPublicCatalogUseCaseTest {

    private val getServerStations: GetServerStationsUseCase = mockk()
    private val preferencesRepository: UserPreferencesRepository = mockk()
    private val useCase = SyncPublicCatalogUseCase(getServerStations, preferencesRepository)

    private fun station(tag: String) = ServerStation.create(
        stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad2$tag",
        displayName = "Posto $tag",
        locationQuality = StationLocationQuality.REVIEWED,
        latitude = -23.55,
        longitude = -46.63,
        cnpjNormalized = "04218406000104",
        municipalityCode = "3550308",
        state = "SP",
        currentRevisionId = null,
    )

    private fun optedIn() {
        coEvery { preferencesRepository.getPreferences() } returns
            UserPreferences(syncPublicCityCatalog = true)
    }

    @Test
    fun `opted out traversal performs no network`() = runTest {
        coEvery { preferencesRepository.getPreferences() } returns UserPreferences()

        val result = useCase.sync()

        assertEquals(0, result.pagesFetched)
        assertFalse(result.complete)
        assertInstanceOf(ServerStationsOutcome.Disabled::class.java, result.outcome)
        coVerify(exactly = 0) { getServerStations(any(), any()) }
    }

    @Test
    fun `single complete page drains`() = runTest {
        optedIn()
        coEvery { getServerStations(20, null) } returns
            ServerStationsOutcome.Fresh(ServerStationPage(listOf(station("a")), null))

        val result = useCase.sync()

        assertEquals(1, result.pagesFetched)
        assertTrue(result.complete)
        coVerify(exactly = 0) { getServerStations.loadMore(any()) }
    }

    @Test
    fun `continuations append until the cursor ends`() = runTest {
        optedIn()
        coEvery { getServerStations(20, null) } returns
            ServerStationsOutcome.Fresh(ServerStationPage(listOf(station("a")), "c1"))
        coEvery { getServerStations.loadMore(20) } returns
            ServerStationsOutcome.Fresh(ServerStationPage(listOf(station("a"), station("b")), null))

        val result = useCase.sync()

        assertEquals(2, result.pagesFetched)
        assertTrue(result.complete)
    }

    @Test
    fun `endless catalog stops at the page cap without completion`() = runTest {
        optedIn()
        coEvery { getServerStations(20, null) } returns
            ServerStationsOutcome.Fresh(ServerStationPage(listOf(station("a")), "c1"))
        coEvery { getServerStations.loadMore(20) } answers {
            ServerStationsOutcome.Fresh(ServerStationPage(listOf(station("a")), "next"))
        }

        val result = useCase.sync()

        assertEquals(SyncPublicCatalogUseCase.MAX_PAGES, result.pagesFetched)
        assertFalse(result.complete)
    }

    @Test
    fun `disabling mid-run stops without completion`() = runTest {
        var calls = 0
        coEvery { preferencesRepository.getPreferences() } answers {
            calls++
            UserPreferences(syncPublicCityCatalog = calls < 2)
        }
        coEvery { getServerStations(20, null) } returns
            ServerStationsOutcome.Fresh(ServerStationPage(listOf(station("a")), "c1"))

        val result = useCase.sync()

        assertEquals(1, result.pagesFetched)
        assertFalse(result.complete)
        coVerify(exactly = 0) { getServerStations.loadMore(any()) }
    }

    @Test
    fun `transport failure keeps partial pages without completion`() = runTest {
        optedIn()
        coEvery { getServerStations(20, null) } returns
            ServerStationsOutcome.Fresh(ServerStationPage(listOf(station("a")), "c1"))
        coEvery { getServerStations.loadMore(20) } returns
            ServerStationsOutcome.StaleCache(
                ServerStationPage(listOf(station("a")), "c1"),
                IOException("down"),
            )

        val result = useCase.sync()

        assertEquals(1, result.pagesFetched)
        assertFalse(result.complete)
        assertInstanceOf(ServerStationsOutcome.StaleCache::class.java, result.outcome)
    }

    @Test
    fun `cancellation aborts the traversal`() = runTest {
        optedIn()
        val release = kotlinx.coroutines.CompletableDeferred<Unit>()
        coEvery { getServerStations(20, null) } returns
            ServerStationsOutcome.Fresh(ServerStationPage(listOf(station("a")), "c1"))
        coEvery { getServerStations.loadMore(20) } coAnswers {
            release.await()
            ServerStationsOutcome.Fresh(ServerStationPage(emptyList(), null))
        }
        val pending = async { useCase.sync() }
        runCurrent()
        pending.cancel()
        release.complete(Unit)
        try {
            pending.await()
            error("expected cancellation")
        } catch (_: kotlinx.coroutines.CancellationException) {
            // Expected: cancelling aborts the traversal.
        }
    }
}
