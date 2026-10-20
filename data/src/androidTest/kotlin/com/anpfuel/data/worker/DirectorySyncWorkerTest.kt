package com.anpfuel.data.worker

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.work.*
import androidx.work.testing.TestListenableWorkerBuilder
import com.anpfuel.application.port.CommunityReadsFlagProvider
import com.anpfuel.application.usecase.directory.GetServerStationsUseCase
import com.anpfuel.application.usecase.directory.SyncPublicCatalogUseCase
import com.anpfuel.domain.discovery.*
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.ServerStationCache
import com.anpfuel.domain.repository.ServerStationGateway
import com.anpfuel.domain.repository.UserPreferencesRepository
import java.io.IOException
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class DirectorySyncWorkerTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private var enabled = true
    private var requests = 0
    private var fail = false
    private var page: ServerStationPage? = null
    private var cursor: String? = null
    private var keepPaging = false
    private var preferences = UserPreferences(
        syncPublicCityCatalog = true,
        preferredMunicipality = "3550308",
    )
    private val preferencesRepository = object : UserPreferencesRepository {
        override suspend fun getPreferences(): UserPreferences = preferences
        override suspend fun savePreferences(updated: UserPreferences) {
            preferences = updated
        }
    }
    private val gateway = object : ServerStationGateway {
        override suspend fun list(limit: Int, cursor: String?): ServerStationPage {
            requests++
            assertEquals(20, limit)
            if (fail) throw IOException("offline")
            val next = if (keepPaging) "cursor-$requests" else this@DirectorySyncWorkerTest.cursor.also {
                this@DirectorySyncWorkerTest.cursor = null
            }
            val items = if (keepPaging) listOf(ServerStation.create("00000000-0000-4000-8000-%012d".format(requests),
                "Synthetic $requests", StationLocationQuality.UNKNOWN, null, null, null, null, null, null)) else emptyList()
            return ServerStationPage(items, next)
        }
        override suspend fun detail(stationId: String): ServerStation = error("no detail downloads")
        override suspend fun nearby(lat: Double, lon: Double, radiusMeters: Int, limit: Int): List<NearbyServerStation> =
            error("unexpected GPS")
    }
    private val scoped = mutableMapOf<String, ServerStationPage>()
    private val cache = object : ServerStationCache {
        override suspend fun saveScopedPage(municipalityCode: String, page: ServerStationPage) {
            scoped[municipalityCode] = page
            if (municipalityCode.endsWith("|download")) this@DirectorySyncWorkerTest.page = page
        }
        override suspend fun loadScopedPage(municipalityCode: String): ServerStationPage? = scoped[municipalityCode]
        override suspend fun removeScopedPage(scope: String) { scoped.remove(scope) }
        override suspend fun savePage(page: ServerStationPage) { this@DirectorySyncWorkerTest.page = page }
        override suspend fun loadPage(): ServerStationPage? = page
        override suspend fun saveDetail(station: ServerStation) = error("no detail downloads")
        override suspend fun loadDetail(stationId: String): ServerStation? = null
        override suspend fun clear() { page = null }
    }
    private fun worker(attempt: Int = 0, city: String? = "3550308"): DirectorySyncWorker {
        val useCase = GetServerStationsUseCase(object : CommunityReadsFlagProvider {
            override fun isEnabled() = enabled
        }, gateway, cache)
        return TestListenableWorkerBuilder<DirectorySyncWorker>(context)
            .setRunAttemptCount(attempt)
            .setInputData(workDataOf(DirectorySyncWorker.KEY_MUNICIPALITY to city))
            .setWorkerFactory(object : WorkerFactory() {
                override fun createWorker(appContext: Context, workerClassName: String, params: WorkerParameters) =
                    DirectorySyncWorker(
                        appContext,
                        params,
                        SyncPublicCatalogUseCase(useCase, preferencesRepository),
                        preferencesRepository,
                    )
            }).build()
    }

    @Test fun successPersistsEmptySnapshotAndStampsSync() = runBlocking {
        assertEquals(ListenableWorker.Result.success(), worker().doWork())
        assertNotNull(page)
        assertEquals(1, requests)
        assertNotNull(preferences.publicCityCatalogLastSyncedAtMillis)
    }
    @Test fun publicCatalogOptOutPerformsNoNetwork() = runBlocking {
        preferences = preferences.copy(syncPublicCityCatalog = false)
        assertEquals(ListenableWorker.Result.success(), worker().doWork())
        assertEquals(0, requests)
        assertNull(page)
        assertNull(preferences.publicCityCatalogLastSyncedAtMillis)
    }
    @Test fun disabledReadsDoNotSync() = runBlocking {
        enabled = false
        assertEquals(ListenableWorker.Result.success(), worker().doWork())
        assertEquals(0, requests)
    }
    @Test fun staleCityScheduleDoesNotDownload() = runBlocking {
        assertEquals(ListenableWorker.Result.success(), worker(city = "5107925").doWork())
        assertEquals(0, requests)
    }
    @Test fun outageRetainsCacheAndRetriesWithBoundedAttempts() = runBlocking {
        page = ServerStationPage(emptyList(), "saved")
        fail = true
        assertEquals(ListenableWorker.Result.retry(), worker().doWork())
        assertEquals("saved", page?.nextCursor)
        assertEquals(ListenableWorker.Result.failure(), worker(3).doWork())
    }
    @Test fun endlessCatalogStopsAtPageCapWithoutCompletionStamp() = runBlocking {
        keepPaging = true
        assertEquals(ListenableWorker.Result.success(), worker().doWork())
        assertEquals(SyncPublicCatalogUseCase.MAX_PAGES, requests)
        assertNull(preferences.publicCityCatalogLastSyncedAtMillis)
    }
    @Test fun wifiPolicySelectsUnmeteredConstraint() {
        val unmetered = DirectorySyncWorker.request("3550308", true)
        assertEquals(NetworkType.UNMETERED, unmetered.workSpec.constraints.requiredNetworkType)
        assertEquals("3550308", unmetered.workSpec.input.getString(DirectorySyncWorker.KEY_MUNICIPALITY))
        val metered = DirectorySyncWorker.request("3550308", false)
        assertEquals(NetworkType.CONNECTED, metered.workSpec.constraints.requiredNetworkType)
        assertEquals(900_000L, unmetered.workSpec.intervalDuration)
        assertEquals(BackoffPolicy.EXPONENTIAL, unmetered.workSpec.backoffPolicy)
        assertEquals(30_000L, unmetered.workSpec.backoffDelayDuration)
    }
}
