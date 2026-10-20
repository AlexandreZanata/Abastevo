package com.anpfuel.app.ui.stations

import androidx.room.Room
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.work.WorkManager
import com.anpfuel.application.port.CommunityReadsFlagProvider
import com.anpfuel.application.usecase.directory.GetServerStationsUseCase
import com.anpfuel.application.usecase.directory.ServerStationsOutcome
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.DirectoryStationHttpClient
import com.anpfuel.data.remote.OkHttpClientFactory
import com.anpfuel.data.repository.DirectoryStationGatewayImpl
import com.anpfuel.data.repository.DirectoryStationRoomCache
import com.anpfuel.data.worker.DirectorySyncWorker
import com.anpfuel.domain.repository.ServerStationGateway
import java.io.IOException
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.launch
import kotlinx.coroutines.flow.first
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

/** Explicit read-only staging smoke; only an isolated in-memory cache is written. */
@RunWith(AndroidJUnit4::class)
class StagingDirectorySyncDeviceTest {
    @Test fun backgroundDirectorySyncIsRegisteredExactlyOnce() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        DirectorySyncWorker.schedule(context, "Synthetic City", true, "SP")
        DirectorySyncWorker.schedule(context, "Synthetic City", true, "SP")
        val work = WorkManager.getInstance(context)
            .getWorkInfosForUniqueWork(DirectorySyncWorker.WORK_NAME).get(10, TimeUnit.SECONDS)
        assertEquals(1, work.size)
        assertFalse(work.single().state.isFinished)
    }

    @Test fun actualValidatedInternetIsObserved() = runBlocking {
        assumeTrue("Explicit live network probe", InstrumentationRegistry.getArguments()
            .getString("directoryLiveStaging") == "true")
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val connected = kotlinx.coroutines.withTimeout(5_000) {
            com.anpfuel.data.repository.NetworkConnectivityRepositoryImpl(context)
                .observeIsConnected().first { it }
        }
        assertTrue(connected)
    }

    @Test fun physicalInternetLossAndReturnReachTheConnectivityObserver() = runBlocking {
        assumeTrue("Explicit physical network transition", InstrumentationRegistry.getArguments()
            .getString("directoryLiveStaging") == "true")
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        fun shell(command: String): String = android.os.ParcelFileDescriptor.AutoCloseInputStream(
            instrumentation.uiAutomation.executeShellCommand(command),
        ).bufferedReader().use { it.readText().trim() }
        val wifi = shell("settings get global wifi_on")
        val mobile = shell("settings get global mobile_data")
        val connected = kotlinx.coroutines.flow.MutableStateFlow<Boolean?>(null)
        val observer = launch(kotlinx.coroutines.Dispatchers.Default) {
            com.anpfuel.data.repository.NetworkConnectivityRepositoryImpl(instrumentation.targetContext)
                .observeIsConnected().collect { connected.value = it }
        }
        try {
            kotlinx.coroutines.withTimeout(10_000) { connected.first { it == true } }
            shell("svc wifi disable")
            shell("svc data disable")
            kotlinx.coroutines.withTimeout(20_000) { connected.first { it == false } }
            if (wifi == "1") shell("svc wifi enable")
            if (mobile == "1") shell("svc data enable")
            kotlinx.coroutines.withTimeout(30_000) { connected.first { it == true } }
            kotlinx.coroutines.withTimeout(15_000) {
                while (shell("settings get global wifi_on") != wifi ||
                    shell("settings get global mobile_data") != mobile
                ) {
                    kotlinx.coroutines.delay(250)
                }
            }
        } finally {
            shell("svc wifi " + if (wifi == "1") "enable" else "disable")
            shell("svc data " + if (mobile == "1") "enable" else "disable")
            observer.cancel()
            observer.join()
        }
    }

    @Test fun actualVpsReadSurvivesOutageAndReconnect() = runBlocking {
        assumeTrue("Explicit live read-only probe", InstrumentationRegistry.getArguments()
            .getString("directoryLiveStaging") == "true")
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val database = Room.inMemoryDatabaseBuilder(context, AnpFuelDatabase::class.java).build()
        try {
            val live = DirectoryStationGatewayImpl(DirectoryStationHttpClient(
                OkHttpClientFactory.create(maxRetries = 0), ApiEnvironment.STAGING.origin,
            ))
            var offline = false
            val gateway = object : ServerStationGateway by live {
                override suspend fun listInMunicipality(municipalityCode: String, limit: Int, cursor: String?) =
                    if (offline) throw IOException("simulated outage") else live.listInMunicipality(municipalityCode, limit, cursor)
            }
            val cache = DirectoryStationRoomCache(database.serverStationCacheDao())
            val useCase = GetServerStationsUseCase(object : CommunityReadsFlagProvider {
                override fun isEnabled() = true
            }, gateway, cache, com.anpfuel.application.port.StationDirectoryScopeProvider { "5107925" })
            val first = useCase(100)
            assertTrue("Actual Android HTTPS read must succeed", first is ServerStationsOutcome.Fresh)
            val ids = (first as ServerStationsOutcome.Fresh).page.items.map { it.stationId }
            assertTrue(ids.isNotEmpty())
            assertTrue(first.page.items.all { it.municipalityCode == "5107925" })
            assertNull(first.page.nextCursor)
            assertEquals(ids.size, ids.toSet().size)
            offline = true
            val cached = useCase(100)
            assertTrue(cached is ServerStationsOutcome.StaleCache)
            assertEquals(ids, (cached as ServerStationsOutcome.StaleCache).page.items.map { it.stationId })
            offline = false
            assertTrue(useCase(100) is ServerStationsOutcome.Fresh)
        } finally { database.close() }
    }
}
