package com.anpfuel.data.local

import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.data.local.entity.ServerCatalogMetaEntity
import com.anpfuel.data.local.entity.ServerStationCacheEntity
import com.anpfuel.data.repository.DirectoryStationRoomCache
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class DirectoryCacheTransactionTest {
    private lateinit var database: AnpFuelDatabase
    private val row = ServerStationCacheEntity(
        stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26", displayName = "Old station",
        locationQuality = "unknown", latitude = null, longitude = null,
        cnpjNormalized = "04218406000104", municipalityCode = "3550308", state = "SP",
        currentRevisionId = null, position = 0, savedAtMillis = 1,
    )
    private fun meta(cursor: String?) = ServerCatalogMetaEntity("page", cursor, 1)

    @Before fun open() {
        database = Room.inMemoryDatabaseBuilder(
            ApplicationProvider.getApplicationContext(), AnpFuelDatabase::class.java,
        ).build()
    }
    @After fun close() = database.close()

    @Test fun failedMetadataWriteRollsBackRowsAndCursor() = runBlocking {
        val dao = database.serverStationCacheDao()
        dao.replacePage(listOf(row), meta("old"))
        database.openHelper.writableDatabase.execSQL(
            "CREATE TRIGGER reject_page BEFORE INSERT ON server_catalog_meta BEGIN SELECT RAISE(ABORT, 'test failure'); END",
        )
        try {
            dao.replacePage(listOf(row.copy(displayName = "New station")), meta("new"))
            fail("metadata failure must abort page replacement")
        } catch (_: android.database.sqlite.SQLiteException) { }
        val (rows, metadata) = dao.pageSnapshot()
        assertEquals("Old station", rows.single().displayName)
        assertEquals("old", metadata?.nextCursor)
    }

    @Test fun successfulEmptySnapshotRemovesOldRowsAndSurvivesNewCacheInstance() = runBlocking {
        val dao = database.serverStationCacheDao()
        dao.replacePage(listOf(row), meta("old"))
        dao.replacePage(emptyList(), meta(null))
        val page = DirectoryStationRoomCache(dao).loadPage()
        assertNotNull(page)
        assertTrue(page!!.items.isEmpty())
        assertNull(page.nextCursor)
    }

    @Test fun refreshRetainsIndividuallyCachedDetailsOutsideThePage() = runBlocking {
        val dao = database.serverStationCacheDao()
        val detail = row.copy(stationId = "e7d85d34-74ec-5d35-b3f6-519dc34ce370", position = Int.MAX_VALUE)
        dao.upsert(detail)
        dao.replacePage(listOf(row), meta(null))
        val cache = DirectoryStationRoomCache(dao)
        assertNotNull(cache.loadDetail(detail.stationId))
        assertEquals(listOf(row.stationId), cache.loadPage()!!.items.map { it.stationId })
    }

    @Test fun scopedPagesAreIsolatedAcrossScopes() = runBlocking {
        val dao = database.serverStationCacheDao()
        val cache = DirectoryStationRoomCache(dao)
        dao.replacePage(listOf(row), meta(null))
        assertNull(cache.loadScopedPage("3550308"))
        val station = cache.loadPage()!!.items.single()
        cache.saveScopedPage("3550308", com.anpfuel.domain.discovery.ServerStationPage(listOf(station), null))
        assertNotNull(cache.loadScopedPage("3550308"))
        assertNull(cache.loadScopedPage("5107925"))
        assertNotNull(cache.loadPage())
        cache.saveScopedPage("5107925", com.anpfuel.domain.discovery.ServerStationPage(emptyList(), null))
        assertNotNull(cache.loadScopedPage("3550308"))
        assertNotNull(cache.loadPage())
        assertTrue(cache.loadScopedPage("5107925")!!.items.isEmpty())
    }

    @Test fun scopedReplacementFailureKeepsItsOwnAndOtherScopes() = runBlocking {
        val dao = database.serverStationCacheDao()
        val cache = DirectoryStationRoomCache(dao)
        dao.replacePage(listOf(row), meta("national"), ServerCatalogMetaEntity.PAGE_KEY)
        cache.saveScopedPage("3550308", com.anpfuel.domain.discovery.ServerStationPage(
            listOf(cache.loadPage()!!.items.single()), "city-cursor"))
        database.openHelper.writableDatabase.execSQL(
            "CREATE TRIGGER reject_scoped_page BEFORE INSERT ON server_catalog_meta BEGIN SELECT RAISE(ABORT, 'test failure'); END",
        )
        try {
            cache.saveScopedPage("3550308", com.anpfuel.domain.discovery.ServerStationPage(emptyList(), null))
            fail("scoped metadata failure must abort page replacement")
        } catch (_: android.database.sqlite.SQLiteException) { }
        assertEquals("city-cursor", cache.loadScopedPage("3550308")?.nextCursor)
        assertEquals("national", cache.loadPage()?.nextCursor)
    }

    @Test fun pagedReadsWindowTheStoredServerOrder() = runBlocking {
        val dao = database.serverStationCacheDao()
        val cache = DirectoryStationRoomCache(dao)
        val rows = (0 until 5).map { index -> row.copy(stationId = "00000000-0000-4000-8000-%012d".format(index), displayName = "Station $index", position = index) }
        dao.replacePage(rows, meta("cursor-5"))
        val first = cache.loadPage(2, 0)
        val second = cache.loadPage(2, 2)
        val tail = cache.loadPage(2, 4)
        assertEquals(listOf("Station 0", "Station 1"), first?.items?.map { it.displayName })
        assertEquals(listOf("Station 2", "Station 3"), second?.items?.map { it.displayName })
        assertEquals(listOf("Station 4"), tail?.items?.map { it.displayName })
        assertEquals("cursor-5", tail?.nextCursor)
    }

    @Test fun concurrentReadersNeverObserveMixedPageAndMetadata() = runBlocking {
        val dao = database.serverStationCacheDao()
        dao.replacePage(listOf(row.copy(displayName = "0")), meta("0"))
        coroutineScope {
            val writer = async {
                repeat(30) { index -> dao.replacePage(listOf(row.copy(displayName = "$index")), meta("$index")) }
            }
            val reader = async {
                repeat(30) {
                    val (rows, metadata) = dao.pageSnapshot()
                    assertEquals(metadata?.nextCursor, rows.single().displayName)
                }
            }
            writer.await()
            reader.await()
        }
    }
}
