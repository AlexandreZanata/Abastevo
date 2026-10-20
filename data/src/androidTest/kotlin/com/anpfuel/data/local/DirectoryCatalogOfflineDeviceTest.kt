package com.anpfuel.data.local

import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.application.port.CommunityReadsFlagProvider
import com.anpfuel.application.port.StationDirectoryScopeProvider
import com.anpfuel.application.usecase.directory.GetServerStationsUseCase
import com.anpfuel.application.usecase.directory.ServerStationsOutcome
import com.anpfuel.data.repository.DirectoryStationRoomCache
import com.anpfuel.domain.discovery.*
import com.anpfuel.domain.repository.ServerStationGateway
import java.util.UUID
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class DirectoryCatalogOfflineDeviceTest {
    @Test fun persistedDownloadPagesOfflineAfterReopenWithoutCrossingOriginAndClearKeepsDetails() = runBlocking {
        val context = ApplicationProvider.getApplicationContext<android.content.Context>()
        val name = "pagination-offline-${UUID.randomUUID()}.db"
        fun open() = Room.databaseBuilder(context, AnpFuelDatabase::class.java, name).build()
        var database = open()
        try {
            val stations = (0 until 125).map { index -> ServerStation.create(
                "00000000-0000-4000-8000-%012d".format(index), "Synthetic $index",
                StationLocationQuality.UNKNOWN, null, null, null, "3550308", "SP", null) }
            var cache = DirectoryStationRoomCache(database.serverStationCacheDao(), "https://one.invalid")
            cache.saveScopedPage("3550308|download", ServerStationPage(stations, null))
            cache.saveDetail(stations.first())
            database.close()
            database = open()
            cache = DirectoryStationRoomCache(database.serverStationCacheDao(), "https://one.invalid")
            val foreign = DirectoryStationRoomCache(database.serverStationCacheDao(), "https://two.invalid")
            assertNull(foreign.loadScopedPage("3550308|download", 20, 0))
            assertNull(foreign.loadDetail(stations.first().stationId))
            var requests = 0
            val gateway = object : ServerStationGateway {
                override suspend fun list(limit: Int, cursor: String?): ServerStationPage { requests++; error("offline") }
                override suspend fun detail(stationId: String): ServerStation = error("unexpected detail IO")
                override suspend fun nearby(lat: Double, lon: Double, radiusMeters: Int, limit: Int): List<NearbyServerStation> = error("unexpected GPS")
            }
            val reads = GetServerStationsUseCase(object : CommunityReadsFlagProvider {
                override fun isEnabled() = true
            }, gateway, cache, StationDirectoryScopeProvider { "3550308" })
            val first = reads.cached() as ServerStationsOutcome.StaleCache
            assertEquals(20, first.page.items.size)
            val ids = first.page.items.map { it.stationId }.toMutableList()
            var offset = first.page.cachedNextOffset
            while (offset != null) {
                val next = reads.loadMore(cachedOffset = offset) as ServerStationsOutcome.Fresh
                assertTrue(next.fromCache)
                assertTrue(next.page.items.size <= 20)
                ids += next.page.items.map { it.stationId }
                offset = next.page.cachedNextOffset
            }
            assertEquals(stations.map { it.stationId }, ids)
            assertEquals(0, requests)
            reads.clearPublicPages()
            assertTrue(reads.cached() is ServerStationsOutcome.Unavailable)
            assertNotNull(cache.loadDetail(stations.first().stationId))
        } finally {
            database.close()
            context.deleteDatabase(name)
        }
    }
}
