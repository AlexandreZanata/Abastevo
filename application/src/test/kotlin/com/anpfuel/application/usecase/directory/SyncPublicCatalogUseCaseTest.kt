package com.anpfuel.application.usecase.directory

import com.anpfuel.application.port.CommunityReadsFlagProvider
import com.anpfuel.application.port.StationDirectoryScopeProvider
import com.anpfuel.domain.discovery.*
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.*
import com.anpfuel.domain.valueobject.BrazilianState
import kotlinx.coroutines.*
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import java.io.IOException

@OptIn(ExperimentalCoroutinesApi::class)
class SyncPublicCatalogUseCaseTest {
    private var preferences = UserPreferences(syncPublicCityCatalog = true, preferredState = BrazilianState.SAO_PAULO, preferredMunicipality = "Synthetic")
    private val prefs = object : UserPreferencesRepository {
        override suspend fun getPreferences() = preferences
        override suspend fun savePreferences(preferences: UserPreferences) { this@SyncPublicCatalogUseCaseTest.preferences = preferences }
    }
    private val pages = mutableMapOf<String, ServerStationPage>()
    private val cache = object : ServerStationCache {
        override suspend fun saveScopedPage(municipalityCode: String, page: ServerStationPage) { pages[municipalityCode] = page }
        override suspend fun loadScopedPage(municipalityCode: String) = pages[municipalityCode]
        override suspend fun removeScopedPage(scope: String) { pages.remove(scope) }
        override suspend fun savePage(page: ServerStationPage) { error("must stay city scoped") }
        override suspend fun loadPage(): ServerStationPage? = null
        override suspend fun saveDetail(station: ServerStation) { error("no detail downloads") }
        override suspend fun loadDetail(stationId: String): ServerStation? = null
        override suspend fun clear() { pages.clear() }
    }
    private var totalPages = 1
    private var failureAt = -1
    private var enabled = true
    private var afterResponse: suspend (Int) -> Unit = {}
    private val cursors = mutableListOf<String?>()
    private val gateway = object : ServerStationGateway {
        override suspend fun list(limit: Int, cursor: String?): ServerStationPage = error("no national downloads")
        override suspend fun listInMunicipality(municipalityCode: String, limit: Int, cursor: String?): ServerStationPage {
            assertEquals("3550308", municipalityCode)
            assertEquals(20, limit)
            cursors += cursor
            val index = cursor?.removePrefix("c")?.toInt() ?: 0
            if (index == failureAt) throw IOException("synthetic outage")
            afterResponse(index)
            return ServerStationPage(listOf(station(index)), if (index + 1 < totalPages) "c${index + 1}" else null)
        }
        override suspend fun detail(stationId: String): ServerStation = error("no detail")
        override suspend fun nearby(lat: Double, lon: Double, radiusMeters: Int, limit: Int): List<NearbyServerStation> = error("no GPS")
    }
    private val reads = GetServerStationsUseCase(object : CommunityReadsFlagProvider { override fun isEnabled() = enabled }, gateway, cache,
        StationDirectoryScopeProvider { "3550308" })
    private val useCase = SyncPublicCatalogUseCase(reads, prefs)
    private fun station(index: Int) = ServerStation.create("00000000-0000-4000-8000-%012d".format(index), "Synthetic $index",
        StationLocationQuality.UNKNOWN, null, null, null, "3550308", "SP", null)

    @Test fun `opted out and disabled reads never download`() = runTest {
        preferences = preferences.copy(syncPublicCityCatalog = false)
        assertFalse(useCase.sync().complete)
        preferences = preferences.copy(syncPublicCityCatalog = true)
        enabled = false
        assertFalse(useCase.sync().complete)
        assertTrue(cursors.isEmpty())
    }
    @Test fun `single page completes and publishes only its city`() = runTest {
        assertTrue(useCase.sync().complete)
        assertEquals(1, pages["3550308|download"]?.items?.size)
        assertEquals(listOf<String?>(null), cursors)
    }
    @Test fun `continuations drain the city without station detail fanout`() = runTest {
        totalPages = 3
        assertEquals(3, useCase.sync().pagesFetched)
        assertEquals(3, pages["3550308|download"]?.items?.size)
    }
    @Test fun `bounded batches resume past one hundred rows instead of restarting`() = runTest {
        totalPages = 7
        val first = useCase.sync()
        assertEquals(5, first.pagesFetched)
        assertFalse(first.complete)
        assertNull(pages["3550308|download"])
        val second = useCase.sync()
        assertTrue(second.complete)
        assertEquals(listOf(null, "c1", "c2", "c3", "c4", "c5", "c6"), cursors)
        assertEquals(7, pages["3550308|download"]?.items?.size)
    }
    @Test fun `outage preserves complete city and partial staging resumes`() = runTest {
        pages["3550308|download"] = ServerStationPage(listOf(station(99)), null)
        totalPages = 3
        failureAt = 1
        assertFalse(useCase.sync().complete)
        assertEquals(station(99).stationId, pages["3550308|download"]?.items?.single()?.stationId)
        failureAt = -1
        assertTrue(useCase.sync().complete)
        assertEquals(listOf(null, "c1", "c1", "c2"), cursors)
    }
    @Test fun `opt out during final response cannot save or certify completion`() = runTest {
        afterResponse = { preferences = preferences.copy(syncPublicCityCatalog = false, publicCityCatalogGeneration = 1) }
        assertFalse(useCase.sync().complete)
        assertTrue(pages.isEmpty())
    }
    @Test fun `city change while awaiting response cannot publish another scope`() = runTest {
        afterResponse = { preferences = preferences.copy(preferredMunicipality = "Other", publicCityCatalogGeneration = 1) }
        assertFalse(useCase.sync().complete)
        assertTrue(pages.isEmpty())
    }
    @Test fun `off and on during response retires the old revision`() = runTest {
        afterResponse = { preferences = preferences.copy(publicCityCatalogGeneration = 2) }
        assertFalse(useCase.sync().complete)
        assertTrue(pages.isEmpty())
    }
    @Test fun `cancellation aborts without committing late page`() = runTest {
        val release = CompletableDeferred<Unit>()
        afterResponse = { withContext(NonCancellable) { release.await() } }
        val request = launch { useCase.sync() }
        runCurrent(); request.cancel(); release.complete(Unit); request.join()
        assertTrue(pages.isEmpty())
    }
    @Test fun `time budget cancels blocked transport and never completes`() = runTest {
        afterResponse = { delay(SyncPublicCatalogUseCase.MAX_RUN_MILLIS + 1) }
        assertFalse(useCase.sync().complete)
        assertTrue(pages.isEmpty())
    }
}
