package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.ServerStationCacheDao
import com.anpfuel.data.local.entity.ServerCatalogMetaEntity
import com.anpfuel.data.local.entity.ServerStationCacheEntity
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.ServerStationPage
import com.anpfuel.domain.discovery.StationLocationQuality
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class DirectoryStationRoomCacheTest {

    private val dao: ServerStationCacheDao = mockk(relaxed = true)

    private fun station() = ServerStation.create(
        stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26",
        displayName = "Posto Central",
        locationQuality = StationLocationQuality.REVIEWED,
        latitude = -23.55,
        longitude = -46.63,
        cnpjNormalized = "04218406000104",
        municipalityCode = "3550308",
        state = "SP",
        currentRevisionId = null,
    )

    private fun entity() = ServerStationCacheEntity(
        stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26",
        displayName = "Posto Central",
        locationQuality = "reviewed",
        latitude = -23.55,
        longitude = -46.63,
        cnpjNormalized = "04218406000104",
        municipalityCode = "3550308",
        state = "SP",
        currentRevisionId = null,
        position = 0,
        savedAtMillis = 1_000_000L,
    )

    @Test
    fun `save replaces page set and load replays in order`() = runTest {
        val repo = DirectoryStationRoomCache(dao)
        val saved = slot<List<ServerStationCacheEntity>>()
        coEvery { dao.upsertAll(capture(saved)) } returns Unit

        repo.savePage(ServerStationPage(listOf(station()), "cursor-1"))

        coVerify { dao.clearStations() }
        coVerify { dao.upsertMeta(any()) }
        assertEquals(1, saved.captured.size)
        assertEquals(0, saved.captured.first().position)
        assertEquals(-23.55, saved.captured.first().latitude)
    }

    @Test
    fun `load replays cached page without network after restart`() = runTest {
        coEvery { dao.loadPage() } returns listOf(entity())
        coEvery { dao.findMeta(ServerCatalogMetaEntity.PAGE_KEY) } returns
            ServerCatalogMetaEntity(ServerCatalogMetaEntity.PAGE_KEY, "cursor-1", 2_000_000L)

        // A fresh instance over the same DAO (simulated restart) replays
        // the last good page: offline compatibility without network.
        val restarted = DirectoryStationRoomCache(dao)
        val page = restarted.loadPage()

        checkNotNull(page)
        assertEquals(1, page.items.size)
        assertEquals("Posto Central", page.items.first().displayName)
        assertEquals("cursor-1", page.nextCursor)
        assertEquals("04218406000104", page.items.first().cnpjNormalized)
    }

    @Test
    fun `empty cache loads null and unknown locations stay honest`() = runTest {
        coEvery { dao.loadPage() } returns emptyList()

        assertNull(DirectoryStationRoomCache(dao).loadPage())

        val unknown = ServerStation.create(
            stationId = "e7d85d34-74ec-5d35-b3f6-519dc34ce370",
            displayName = "[P34-TEST] Posto Gama",
            locationQuality = StationLocationQuality.UNKNOWN,
            latitude = null,
            longitude = null,
            cnpjNormalized = "12ABC34501DE35",
            municipalityCode = "3550308",
            state = "SP",
            currentRevisionId = null,
        )
        val saved = slot<ServerStationCacheEntity>()
        coEvery { dao.findById(any()) } returns null
        coEvery { dao.upsert(capture(saved)) } returns Unit

        DirectoryStationRoomCache(dao).saveDetail(unknown)

        assertNull(saved.captured.latitude)
        assertNull(saved.captured.longitude)
    }

    @Test
    fun `detail resolves case-insensitively and clear wipes page`() = runTest {
        coEvery { dao.findById("d6c74c23-63db-4c24-a2e5-408cb23bad26") } returns entity()

        val repo = DirectoryStationRoomCache(dao)
        val detail = repo.loadDetail("D6C74C23-63DB-4C24-A2E5-408CB23BAD26")

        checkNotNull(detail)
        assertEquals("Posto Central", detail.displayName)

        repo.clear()
        coVerify { dao.clearStations() }
        coVerify { dao.clearMeta() }
    }
}
