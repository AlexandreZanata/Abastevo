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
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class DirectoryStationRoomCacheTest {

    private val dao: ServerStationCacheDao = mockk(relaxed = true)

    private fun station(suffix: String = "") = ServerStation.create(
        stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26$suffix",
        displayName = "Posto Central",
        locationQuality = StationLocationQuality.REVIEWED,
        latitude = -23.55,
        longitude = -46.63,
        cnpjNormalized = "04218406000104",
        municipalityCode = "3550308",
        state = "SP",
        currentRevisionId = null,
        address = mapOf("street" to "Synthetic Street"),
    )

    private fun entity(scope: String = ServerCatalogMetaEntity.PAGE_KEY) = ServerStationCacheEntity(
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
        addressJson = "{\"street\":\"Synthetic Street\"}",
        scopeKey = scope,
    )

    @Test
    fun `save replaces page set and load replays in order`() = runTest {
        val repo = DirectoryStationRoomCache(dao)
        val saved = slot<List<ServerStationCacheEntity>>()
        coEvery { dao.replacePage(capture(saved), any(), any()) } returns Unit

        repo.savePage(ServerStationPage(listOf(station()), "cursor-1"))

        coVerify { dao.replacePage(any(), any(), ServerCatalogMetaEntity.PAGE_KEY) }
        assertEquals(1, saved.captured.size)
        assertEquals(0, saved.captured.first().position)
        assertEquals(ServerCatalogMetaEntity.PAGE_KEY, saved.captured.first().scopeKey)
        assertEquals(-23.55, saved.captured.first().latitude)
        assertEquals("Synthetic Street",org.json.JSONObject(saved.captured.first().addressJson).getString("street"))
    }

    @Test
    fun `scoped save never touches the national scope`() = runTest {
        val repo = DirectoryStationRoomCache(dao)
        coEvery { dao.replacePage(any(), any(), any()) } returns Unit

        repo.saveScopedPage("3550308", ServerStationPage(listOf(station()), "cursor-2"))

        coVerify { dao.replacePage(any(), any(), "city:3550308") }
        coVerify(exactly = 0) { dao.replacePage(any(), any(), ServerCatalogMetaEntity.PAGE_KEY) }
        coVerify(exactly = 0) { dao.clearStations() }
        coVerify(exactly = 0) { dao.clearMeta() }
    }

    @Test
    fun `load replays cached page without network after restart`() = runTest {
        coEvery { dao.pageSnapshot(ServerCatalogMetaEntity.PAGE_KEY) } returns (listOf(entity()) to
            ServerCatalogMetaEntity(ServerCatalogMetaEntity.PAGE_KEY, "cursor-1", 2_000_000L))

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
    fun `scoped load reads its own scope key`() = runTest {
        coEvery { dao.pageSnapshot("city:3550308") } returns (listOf(entity("city:3550308")) to
            ServerCatalogMetaEntity("city:3550308", null, 2_000_000L))

        val page = DirectoryStationRoomCache(dao).loadScopedPage("3550308")

        checkNotNull(page)
        assertEquals(1, page.items.size)
        coVerify { dao.pageSnapshot("city:3550308") }
    }

    @Test
    fun `paged reads window the stored order`() = runTest {
        coEvery { dao.pagedSnapshot(ServerCatalogMetaEntity.PAGE_KEY, 20, 20) } returns (listOf(entity()) to
            ServerCatalogMetaEntity(ServerCatalogMetaEntity.PAGE_KEY, "cursor-9", 3_000_000L))
        coEvery { dao.pagedSnapshot("city:3550308", 20, 0) } returns (emptyList<ServerStationCacheEntity>() to
            ServerCatalogMetaEntity("city:3550308", null, 3_000_000L))

        val repo = DirectoryStationRoomCache(dao)
        val window = repo.loadPage(20, 20)
        checkNotNull(window)
        assertEquals(1, window.items.size)
        assertEquals("cursor-9", window.nextCursor)
        assertEquals(emptyList<ServerStation>(), repo.loadScopedPage("3550308", 20, 0)?.items)
    }

    @Test
    fun `invalid windows are refused before IO`() = runTest {
        val repo = DirectoryStationRoomCache(dao)

        assertThrows(IllegalArgumentException::class.java) { kotlinx.coroutines.runBlocking { repo.loadPage(0, 0) } }
        assertThrows(IllegalArgumentException::class.java) { kotlinx.coroutines.runBlocking { repo.loadPage(20, -1) } }

        coVerify(exactly = 0) { dao.pagedSnapshot(any(), any(), any()) }
    }

    @Test
    fun `empty cache loads null and unknown locations stay honest`() = runTest {
        coEvery { dao.pageSnapshot(any()) } returns (emptyList<ServerStationCacheEntity>() to null)

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
        assertEquals(ServerStationCacheEntity.DETAIL_SCOPE, saved.captured.scopeKey)
        assertEquals(Int.MAX_VALUE, saved.captured.position)
    }

    @Test
    fun `successful empty response is a valid offline snapshot`() = runTest {
        coEvery { dao.pageSnapshot(any()) } returns (emptyList<ServerStationCacheEntity>() to
            ServerCatalogMetaEntity(ServerCatalogMetaEntity.PAGE_KEY, null, 2_000_000L))
        assertEquals(emptyList<ServerStation>(), DirectoryStationRoomCache(dao).loadPage()?.items)
    }

    @Test
    fun `detail fetched outside page never leaks into list`() = runTest {
        coEvery { dao.pageSnapshot(any()) } returns (listOf(entity().copy(position = Int.MAX_VALUE)) to
            ServerCatalogMetaEntity(ServerCatalogMetaEntity.PAGE_KEY, null, 2_000_000L))
        assertEquals(emptyList<ServerStation>(), DirectoryStationRoomCache(dao).loadPage()?.items)
    }

    @Test
    fun `clearing the public catalog drops pages but keeps details`() = runTest {
        val repo = DirectoryStationRoomCache(dao)
        coEvery { dao.clearPublicCatalog() } returns Unit

        repo.clearPublicPages()

        coVerify { dao.clearPublicCatalog() }
        coVerify(exactly = 0) { dao.clearStations() }
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
