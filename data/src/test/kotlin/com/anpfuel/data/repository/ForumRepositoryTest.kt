package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.ForumCacheDao
import com.anpfuel.data.local.entity.ForumCacheEntity
import com.anpfuel.data.local.forum.ForumCacheCodec
import com.anpfuel.data.local.forum.ForumCacheRule
import com.anpfuel.domain.forum.ForumCategoriesResult
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumCategoryItem
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumFeedFilters
import com.anpfuel.domain.forum.ForumFeedPage
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.forum.ForumPostKind
import com.anpfuel.domain.forum.ForumPostSummary
import com.anpfuel.domain.forum.ForumRegion
import com.anpfuel.domain.forum.ForumSourceAttribution
import com.anpfuel.domain.repository.ForumGateway
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ForumRepositoryTest {

    private val origin = "https://teste.abastevo.com.br"
    private val otherOrigin = "https://preview.abastevo.com.br"
    private var testTime = 1_700_000_000_000L

    private val sampleCategories = ForumCategoriesResult(
        items = listOf(
            ForumCategoryItem(ForumCategory.FUELS, "FUELS", "Combustíveis", "Preços"),
        ),
        taxonomyVersion = 1,
    )

    private val sampleSummary = ForumPostSummary(
        id = "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
        kind = ForumPostKind.EDITORIAL,
        rawKind = "EDITORIAL",
        category = ForumCategory.FUELS,
        rawCategory = "FUELS",
        title = "Artigo 1",
        excerpt = "Resumo 1",
        source = ForumSourceAttribution("Redação", "https://abastevo.com.br", "Abastevo", null),
        publicAlias = null,
        firstPublishedAt = "2026-11-05T12:00:00Z",
        updatedAt = "2026-11-05T12:00:00Z",
        revision = 1L,
        commentsEnabled = true,
        region = ForumRegion("SP", 3550308),
        fuelProduct = "ETANOL",
        stationId = null,
    )

    private val sampleFeed = ForumFeedPage(
        items = listOf(sampleSummary),
        nextCursor = "cursor-next-page",
        snapshot = "rev-1",
        fetchedAt = "2026-11-05T12:00:00Z",
    )

    private val sampleDetail = ForumPostDetail(
        id = sampleSummary.id,
        kind = sampleSummary.kind,
        rawKind = sampleSummary.rawKind,
        category = sampleSummary.category,
        rawCategory = sampleSummary.rawCategory,
        title = sampleSummary.title,
        excerpt = sampleSummary.excerpt,
        bodyText = "Corpo completo detalhado do artigo 1.",
        source = sampleSummary.source,
        publicAlias = null,
        sourcePublishedAt = null,
        firstPublishedAt = sampleSummary.firstPublishedAt,
        updatedAt = sampleSummary.updatedAt,
        revision = 1L,
        commentsEnabled = true,
        correction = null,
        region = sampleSummary.region,
        fuelProduct = "ETANOL",
        stationId = null,
    )

    private class FakeForumCacheDao : ForumCacheDao {
        val rows = mutableMapOf<Pair<String, String>, ForumCacheEntity>()

        override suspend fun findByKey(origin: String, key: String): ForumCacheEntity? {
            return rows[origin to key]
        }

        override suspend fun upsert(entity: ForumCacheEntity) {
            rows[entity.origin to entity.key] = entity
        }

        override suspend fun deleteByKey(origin: String, key: String) {
            rows.remove(origin to key)
        }

        override suspend fun deleteOlderThan(notBeforeMillis: Long) {
            rows.entries.removeIf { it.value.fetchedAtMillis < notBeforeMillis }
        }

        override suspend fun trimFeedScopes(origin: String, maxScopes: Int) {
            val feedScopes = rows.entries
                .filter { it.key.first == origin && it.value.kind == ForumCacheRule.KIND_FEED }
                .sortedByDescending { it.value.fetchedAtMillis }

            if (feedScopes.size > maxScopes) {
                val toRemove = feedScopes.drop(maxScopes)
                for (entry in toRemove) {
                    rows.remove(entry.key)
                }
            }
        }

        override suspend fun totalBytes(origin: String): Long {
            return rows.entries
                .filter { it.key.first == origin }
                .sumOf { it.value.payloadBytes }
        }

        override suspend fun deleteOldest(origin: String, count: Int) {
            val oldest = rows.entries
                .filter { it.key.first == origin }
                .sortedBy { it.value.fetchedAtMillis }
                .take(count)

            for (entry in oldest) {
                rows.remove(entry.key)
            }
        }

        override suspend fun count(origin: String): Int {
            return rows.keys.count { it.first == origin }
        }

        override suspend fun clearOrigin(origin: String) {
            rows.entries.removeIf { it.key.first == origin }
        }
    }

    private class FakeForumGateway(
        var categoriesResult: ForumCategoriesResult? = null,
        var feedPage: ForumFeedPage? = null,
        var detailResult: ForumPostDetail? = null,
        var errorToThrow: Exception? = null,
    ) : ForumGateway {
        var recordedLimit: Int = 0
        var recordedCursor: String? = null

        override suspend fun getCategories(): ForumCategoriesResult {
            errorToThrow?.let { throw it }
            return categoriesResult ?: throw ForumException.Unavailable("no categories")
        }

        override suspend fun getFeed(
            category: ForumCategory?,
            kind: ForumPostKind?,
            limit: Int,
            cursor: String?,
            filters: ForumFeedFilters,
        ): ForumFeedPage {
            recordedLimit = limit
            recordedCursor = cursor
            errorToThrow?.let { throw it }
            return feedPage ?: throw ForumException.Unavailable("no feed")
        }

        override suspend fun getPostDetail(postId: String): ForumPostDetail {
            errorToThrow?.let { throw it }
            return detailResult ?: throw ForumException.NotFound("not found")
        }
    }

    @Test
    fun `getCategories caches online response and falls back when offline`() = runTest {
        val dao = FakeForumCacheDao()
        val gateway = FakeForumGateway(categoriesResult = sampleCategories)
        val repository = ForumRepository(gateway, dao, origin, clock = { testTime })

        // Online fetch
        val onlineResult = repository.getCategories()
        assertEquals(1, onlineResult.items.size)
        assertEquals(1, dao.rows.size)

        // Turn network offline
        gateway.errorToThrow = ForumException.Offline("forum.offline")
        val offlineResult = repository.getCategories()
        assertEquals(1, offlineResult.items.size)
        assertEquals(ForumCategory.FUELS, offlineResult.items[0].code)
    }

    @Test
    fun `getFeed caches snapshot and offline returns page with null nextCursor`() = runTest {
        val dao = FakeForumCacheDao()
        val gateway = FakeForumGateway(feedPage = sampleFeed)
        val repository = ForumRepository(gateway, dao, origin, clock = { testTime })

        // Online fetch
        val onlineFeed = repository.getFeed(limit = 20)
        assertEquals(1, onlineFeed.items.size)
        assertEquals("cursor-next-page", onlineFeed.nextCursor)

        // Offline fetch
        gateway.errorToThrow = ForumException.Offline("forum.offline")
        val offlineFeed = repository.getFeed(limit = 20)
        assertEquals(1, offlineFeed.items.size)
        // Next cursor is intentionally stripped offline to prevent invalid paging attempts
        assertNull(offlineFeed.nextCursor)
        assertEquals("rev-1", offlineFeed.snapshot)
    }

    @Test
    fun `feed cache scope includes subject filters`() = runTest {
        val dao = FakeForumCacheDao()
        val gateway = FakeForumGateway(feedPage = sampleFeed)
        val repository = ForumRepository(gateway, dao, origin, clock = { testTime })

        repository.getFeed(limit = 20, filters = ForumFeedFilters(uf = "SP"))
        repository.getFeed(limit = 20, filters = ForumFeedFilters(uf = "RJ"))
        repository.getFeed(limit = 20, filters = ForumFeedFilters(uf = "SP", fuelProduct = "ETHANOL"))

        assertEquals(
            3,
            dao.rows.values.count { it.kind == ForumCacheRule.KIND_FEED },
        )

        gateway.errorToThrow = ForumException.Offline("forum.offline")
        val offline = repository.getFeed(limit = 20, filters = ForumFeedFilters(uf = "SP"))
        assertEquals("rev-1", offline.snapshot)
        assertNull(offline.nextCursor)
    }

    @Test
    fun `getPostDetail caches detail and falls back when offline`() = runTest {
        val dao = FakeForumCacheDao()
        val gateway = FakeForumGateway(detailResult = sampleDetail)
        val repository = ForumRepository(gateway, dao, origin, clock = { testTime })

        val onlineDetail = repository.getPostDetail(sampleDetail.id)
        assertEquals(sampleDetail.id, onlineDetail.id)
        assertEquals("Corpo completo detalhado do artigo 1.", onlineDetail.bodyText)

        // Offline fallback
        gateway.errorToThrow = ForumException.Offline("forum.offline")
        val offlineDetail = repository.getPostDetail(sampleDetail.id)
        assertEquals(sampleDetail.id, offlineDetail.id)
        assertEquals(onlineDetail.title, offlineDetail.title)
    }

    @Test
    fun `live 404 invalidates post detail cache immediately`() = runTest {
        val dao = FakeForumCacheDao()
        val gateway = FakeForumGateway(detailResult = sampleDetail)
        val repository = ForumRepository(gateway, dao, origin, clock = { testTime })

        // Cache post detail
        repository.getPostDetail(sampleDetail.id)
        assertNotNull(dao.findByKey(origin, "post:${sampleDetail.id}"))

        // Next call returns 404 (post was hidden/unpublished)
        gateway.errorToThrow = ForumException.NotFound("forum.not-found")
        assertThrows(ForumException.NotFound::class.java) {
            kotlinx.coroutines.runBlocking { repository.getPostDetail(sampleDetail.id) }
        }

        // Cache must have been cleared
        assertNull(dao.findByKey(origin, "post:${sampleDetail.id}"))
    }

    @Test
    fun `entries older than 24h expire and are not returned offline`() = runTest {
        val dao = FakeForumCacheDao()
        val gateway = FakeForumGateway(categoriesResult = sampleCategories)
        val repository = ForumRepository(gateway, dao, origin, clock = { testTime })

        repository.getCategories()

        // Fast forward 25 hours (25 * 3600 * 1000 ms)
        testTime += 25 * 3600 * 1000L
        gateway.errorToThrow = ForumException.Offline("forum.offline")

        assertThrows(ForumException.Offline::class.java) {
            kotlinx.coroutines.runBlocking { repository.getCategories() }
        }
    }

    @Test
    fun `enforces five-scope limit on feed cache`() = runTest {
        val dao = FakeForumCacheDao()
        val gateway = FakeForumGateway(feedPage = sampleFeed)
        val repository = ForumRepository(gateway, dao, origin, clock = { testTime })

        val categories = listOf(
            ForumCategory.FUELS,
            ForumCategory.ELECTRIC,
            ForumCategory.TRAFFIC_RULES,
            ForumCategory.VEHICLES_TECH,
            ForumCategory.ROADS_MOBILITY,
            ForumCategory.DRIVER_ECONOMY, // 6th scope
        )

        for ((idx, cat) in categories.withIndex()) {
            testTime += 1000L * (idx + 1)
            repository.getFeed(category = cat, limit = 20)
        }

        val feedEntries = dao.rows.values.filter { it.kind == ForumCacheRule.KIND_FEED }
        assertEquals(5, feedEntries.size)
    }

    @Test
    fun `enforces 2 MiB byte limit eviction`() = runTest {
        val dao = FakeForumCacheDao()
        val gateway = FakeForumGateway()
        val repository = ForumRepository(gateway, dao, origin, clock = { testTime })

        // Insert large payloads totaling > 2 MiB
        val largePayload = "a".repeat(200 * 1024) // 200 KiB
        for (i in 1..12) { // 12 * 200 KiB = 2.4 MiB > 2 MiB cap
            testTime += 1000L * i
            val detail = sampleDetail.copy(id = "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb$i", bodyText = largePayload)
            gateway.detailResult = detail
            repository.getPostDetail(detail.id)
        }

        val totalBytes = dao.totalBytes(origin)
        assertTrue(totalBytes <= ForumCacheRule.MAX_TOTAL_BYTES, "total bytes $totalBytes <= 2 MiB")
    }

    @Test
    fun `origin separation prevents cross-origin cache leakage`() = runTest {
        val dao = FakeForumCacheDao()
        val gateway = FakeForumGateway(categoriesResult = sampleCategories)

        val repo1 = ForumRepository(gateway, dao, origin, clock = { testTime })
        val repo2 = ForumRepository(gateway, dao, otherOrigin, clock = { testTime })

        repo1.getCategories()
        assertEquals(1, dao.count(origin))
        assertEquals(0, dao.count(otherOrigin))

        // Offline on repo2 should not see repo1's cache
        gateway.errorToThrow = ForumException.Offline("forum.offline")
        assertThrows(ForumException.Offline::class.java) {
            kotlinx.coroutines.runBlocking { repo2.getCategories() }
        }
    }

    @Test
    fun `disabled forum error does not fallback to stale cache`() = runTest {
        val dao = FakeForumCacheDao()
        val gateway = FakeForumGateway(categoriesResult = sampleCategories)
        val repository = ForumRepository(gateway, dao, origin, clock = { testTime })

        repository.getCategories()

        // Server reports forum is disabled
        gateway.errorToThrow = ForumException.Disabled("forum.disabled")
        val ex = assertThrows(ForumException.Disabled::class.java) {
            kotlinx.coroutines.runBlocking { repository.getCategories() }
        }
        assertEquals("forum.disabled", ex.code)
    }
}
