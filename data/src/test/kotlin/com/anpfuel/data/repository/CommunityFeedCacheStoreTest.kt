package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.CommunityFeedCacheDao
import com.anpfuel.data.local.entity.CommunityFeedCacheEntity
import com.anpfuel.domain.community.CityCommunityFeedReader
import com.anpfuel.domain.community.CityCommunitySummary
import com.anpfuel.domain.community.CityCommunitySummaryReader
import com.anpfuel.domain.community.CommunityFeedCacheRule
import com.anpfuel.domain.community.CommunityFeedItem
import com.anpfuel.domain.community.CommunityFeedPage
import com.anpfuel.domain.community.CommunitySummaryPrice
import com.anpfuel.domain.community.FeedCity
import com.anpfuel.domain.community.FeedQuery
import com.anpfuel.domain.community.FeedSort
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import java.io.IOException
import java.time.Instant
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class CommunityFeedCacheStoreTest {
    private val city = FeedCity("5103403", BrazilianState.MATO_GROSSO, "Synthetic city")
    private val otherCity = FeedCity("5107925", BrazilianState.MATO_GROSSO, "Other city")
    private val at = Instant.parse("2026-10-14T12:00:00Z")
    private val origin = "https://teste.abastevo.com.br"
    private var nowMillis = at.toEpochMilli()

    private fun item(
        price: Long = 5032,
        expiresAt: Instant = at.plusSeconds(3600),
        stationId: String = "00000000-0000-0000-0000-000000000001",
    ) = CommunityFeedItem(
        stationId = stationId,
        stationName = "Synthetic station",
        fuel = FuelProduct.ETHANOL,
        amountMilliBrl = price,
        updatedAt = at.minusSeconds(60),
        expiresAt = expiresAt,
        supporters = 1,
        confirmations = 0,
        confidence = "LOW",
        version = 1,
    )

    private fun page() = CommunityFeedPage(listOf(item()), null, at)

    private fun summary() = CityCommunitySummary(
        city,
        listOf(CommunitySummaryPrice(FuelProduct.ETHANOL, 5032, 5000, 5064, 65, at.minusSeconds(60), at.plusSeconds(3600))),
        at,
    )

    private class FakeDao : CommunityFeedCacheDao {
        val rows = mutableMapOf<String, CommunityFeedCacheEntity>()
        var failReads = false
        var failWrites = false
        var pruned = false
        override suspend fun findByKey(key: String): CommunityFeedCacheEntity? {
            if (failReads) throw IOException("db down")
            return rows[key]
        }
        override suspend fun upsert(entity: CommunityFeedCacheEntity) {
            if (failWrites) throw IOException("db down")
            rows[entity.key] = entity
        }
        override suspend fun deleteByKey(key: String) {
            rows.remove(key)
        }
        override suspend fun deleteOlderThan(notBeforeMillis: Long) {
            rows.entries.removeIf { it.value.fetchedAtMillis < notBeforeMillis }
            pruned = true
        }
        override suspend fun trimToNewest(maxEntries: Int) {
            val drop = rows.entries.sortedBy { it.value.fetchedAtMillis }.dropLast(maxEntries)
            drop.forEach { rows.remove(it.key) }
        }
        override suspend fun count(): Int = rows.size
    }

    private fun store(
        dao: CommunityFeedCacheDao,
        feed: CityCommunityFeedReader = mockk(),
        summary: CityCommunitySummaryReader = mockk(),
        origin: String = this.origin,
    ) = CommunityFeedCacheStore(feed, summary, dao, origin, nowMillis = { nowMillis })

    @Test fun `first page success writes scoped snapshot and prunes`() = runTest {
        val dao = FakeDao()
        val feed = mockk<CityCommunityFeedReader>()
        coEvery { feed.read(any(), null) } returns page()
        val store = store(dao, feed = feed)

        assertEquals(5032L, store.read(FeedQuery(city, FuelProduct.ETHANOL, FeedSort.RECENT), null).items.single().amountMilliBrl)

        val key = CommunityFeedCacheRule.feedKey(
            com.anpfuel.domain.community.CommunityFeedScope(origin, city, FuelProduct.ETHANOL, FeedSort.RECENT),
        )
        val saved = dao.rows[key]
        checkNotNull(saved)
        assertEquals("MT", saved.state)
        assertEquals("5103403", saved.municipalityCode)
        assertTrue(saved.payloadJson.length <= CommunityFeedCacheRule.MAX_PAYLOAD_CHARS)
        assertTrue(dao.pruned)
        // The stored payload round-trips through the strict decoder.
        assertEquals(5032L, store.snapshotFeed(city, FuelProduct.ETHANOL, FeedSort.RECENT)!!.items.single().amountMilliBrl)
    }

    @Test fun `cursor pages bypass the cache entirely`() = runTest {
        val dao = FakeDao()
        val feed = mockk<CityCommunityFeedReader>()
        coEvery { feed.read(any(), "page-two") } returns page()
        val store = store(dao, feed = feed)

        store.read(FeedQuery(city, FuelProduct.ETHANOL, FeedSort.RECENT), "page-two")

        assertTrue(dao.rows.isEmpty())
        coVerify(exactly = 1) { feed.read(any(), "page-two") }
    }

    @Test fun `network failure never overwrites a snapshot`() = runTest {
        val dao = FakeDao()
        val feed = mockk<CityCommunityFeedReader>()
        coEvery { feed.read(any(), null) } returns page()
        val store = store(dao, feed = feed)
        store.read(FeedQuery(city, FuelProduct.ETHANOL, FeedSort.RECENT), null)
        val before = dao.rows.values.single().payloadJson

        coEvery { feed.read(any(), null) } throws IOException("community feed unavailable")
        try {
            store.read(FeedQuery(city, FuelProduct.ETHANOL, FeedSort.RECENT), null)
            fail("expected community feed failure")
        } catch (_: IOException) {
        }
        assertEquals(before, dao.rows.values.single().payloadJson)
    }

    @Test fun `cache write failure never fails a validated read`() = runTest {
        val dao = FakeDao().apply { failWrites = true }
        val feed = mockk<CityCommunityFeedReader>()
        coEvery { feed.read(any(), null) } returns page()

        assertEquals(5032L, store(dao, feed = feed).read(FeedQuery(city, FuelProduct.ETHANOL, FeedSort.RECENT), null).items.single().amountMilliBrl)
    }

    @Test fun `snapshot filters expired rows but keeps its age`() = runTest {
        val dao = FakeDao()
        val feed = mockk<CityCommunityFeedReader>()
        val mixed = CommunityFeedPage(
            listOf(
                item(5032, at.plusSeconds(60), "00000000-0000-0000-0000-000000000001"),
                item(4999, at.plusSeconds(3600), "00000000-0000-0000-0000-000000000002"),
            ),
            null,
            at,
        )
        coEvery { feed.read(any(), null) } returns mixed
        val store = store(dao, feed = feed)
        store.read(FeedQuery(city, FuelProduct.ETHANOL, FeedSort.RECENT), null)

        nowMillis = at.plusSeconds(120).toEpochMilli()
        val snapshot = store.snapshotFeed(city, FuelProduct.ETHANOL, FeedSort.RECENT)
        checkNotNull(snapshot)
        assertEquals(listOf(4999L), snapshot.items.map { it.amountMilliBrl })
        assertEquals(at, snapshot.generatedAt)

        nowMillis = at.plusSeconds(7200).toEpochMilli()
        val allExpired = store.snapshotFeed(city, FuelProduct.ETHANOL, FeedSort.RECENT)
        checkNotNull(allExpired)
        assertTrue(allExpired.items.isEmpty())
        assertEquals(at, allExpired.generatedAt)
    }

    @Test fun `missing corrupt or oversized snapshots return null`() = runTest {
        val dao = FakeDao()
        val store = store(dao)
        assertNull(store.snapshotFeed(city, FuelProduct.ETHANOL, FeedSort.RECENT))
        assertNull(store.snapshotSummary(city))

        val key = CommunityFeedCacheRule.feedKey(
            com.anpfuel.domain.community.CommunityFeedScope(origin, city, FuelProduct.ETHANOL, FeedSort.RECENT),
        )
        dao.rows[key] = CommunityFeedCacheEntity(
            key, "FEED", origin, "MT", "5103403", "ETHANOL", "RECENT", "{corrupt", at.toEpochMilli(), nowMillis,
        )
        assertNull(store.snapshotFeed(city, FuelProduct.ETHANOL, FeedSort.RECENT))

        dao.rows[key] = dao.rows.getValue(key).copy(payloadJson = "x".repeat(CommunityFeedCacheRule.MAX_PAYLOAD_CHARS + 1))
        assertNull(store.snapshotFeed(city, FuelProduct.ETHANOL, FeedSort.RECENT))

        dao.failReads = true
        assertNull(store.snapshotFeed(city, FuelProduct.ETHANOL, FeedSort.RECENT))
    }

    @Test fun `origin isolates snapshots`() = runTest {
        val dao = FakeDao()
        val feed = mockk<CityCommunityFeedReader>()
        coEvery { feed.read(any(), null) } returns page()
        store(dao, feed = feed, origin = origin).read(FeedQuery(city, FuelProduct.ETHANOL, FeedSort.RECENT), null)

        val other = store(dao, origin = "https://other.invalid")
        assertNull(other.snapshotFeed(city, FuelProduct.ETHANOL, FeedSort.RECENT))
        assertNull(other.snapshotSummary(city))
        assertEquals(1, dao.count())
    }

    @Test fun `summary success writes and failure rethrows without writing`() = runTest {
        val dao = FakeDao()
        val summaryReader = mockk<CityCommunitySummaryReader>()
        coEvery { summaryReader.readSummary(city) } returns summary()
        val store = store(dao, summary = summaryReader)

        assertEquals(65, store.readSummary(city).items.single().stationCount)
        val snapshot = store.snapshotSummary(city)
        checkNotNull(snapshot)
        assertEquals(65, snapshot.items.single().stationCount)

        coEvery { summaryReader.readSummary(otherCity) } throws IOException("community summary unavailable")
        try {
            store.readSummary(otherCity)
            fail("expected community summary failure")
        } catch (_: IOException) {
        }
        assertEquals(1, dao.count())
    }
}
