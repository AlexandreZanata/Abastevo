package com.anpfuel.data.worker

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.work.*
import androidx.work.testing.TestListenableWorkerBuilder
import com.anpfuel.application.port.CommunityReadsFlagProvider
import com.anpfuel.application.usecase.directory.GetServerStationsUseCase
import com.anpfuel.domain.discovery.*
import com.anpfuel.domain.repository.ServerStationCache
import com.anpfuel.domain.repository.ServerStationGateway
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
    private val gateway = object : ServerStationGateway {
        override suspend fun list(limit: Int, cursor: String?): ServerStationPage {
            requests++
            assertEquals(100, limit)
            assertNull(cursor)
            if (fail) throw IOException("offline")
            return ServerStationPage(emptyList(), null)
        }
        override suspend fun detail(stationId: String): ServerStation = error("unexpected detail")
        override suspend fun nearby(lat: Double, lon: Double, radiusMeters: Int, limit: Int): List<NearbyServerStation> =
            error("unexpected GPS")
    }
    private val cache = object : ServerStationCache {
        override suspend fun savePage(page: ServerStationPage) { this@DirectorySyncWorkerTest.page = page }
        override suspend fun loadPage(): ServerStationPage? = page
        override suspend fun saveDetail(station: ServerStation) = error("unexpected detail")
        override suspend fun loadDetail(stationId: String): ServerStation? = null
        override suspend fun clear() { page = null }
    }
    private fun worker(attempt: Int = 0): DirectorySyncWorker {
        val useCase = GetServerStationsUseCase(object : CommunityReadsFlagProvider {
            override fun isEnabled() = enabled
        }, gateway, cache)
        return TestListenableWorkerBuilder<DirectorySyncWorker>(context)
            .setRunAttemptCount(attempt)
            .setWorkerFactory(object : WorkerFactory() {
                override fun createWorker(appContext: Context, workerClassName: String, params: WorkerParameters) =
                    DirectorySyncWorker(appContext, params, useCase)
            }).build()
    }

    @Test fun successPersistsEmptySnapshot() = runBlocking {
        assertEquals(ListenableWorker.Result.success(), worker().doWork())
        assertNotNull(page)
        assertEquals(1, requests)
    }
    @Test fun disabledDoesNotReadNetwork() = runBlocking {
        enabled = false
        assertEquals(ListenableWorker.Result.success(), worker().doWork())
        assertEquals(0, requests)
    }
    @Test fun outageRetainsCacheAndRetriesWithBoundedAttempts() = runBlocking {
        page = ServerStationPage(emptyList(), "saved")
        fail = true
        assertEquals(ListenableWorker.Result.retry(), worker().doWork())
        assertEquals("saved", page?.nextCursor)
        assertEquals(ListenableWorker.Result.failure(), worker(3).doWork())
    }
    @Test fun periodicRequestRequiresInternetAndExponentialBackoff() {
        val request = DirectorySyncWorker.request()
        assertEquals(NetworkType.CONNECTED, request.workSpec.constraints.requiredNetworkType)
        assertEquals(900_000L, request.workSpec.intervalDuration)
        assertEquals(BackoffPolicy.EXPONENTIAL, request.workSpec.backoffPolicy)
        assertEquals(30_000L, request.workSpec.backoffDelayDuration)
    }
}
