package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.ForumCacheDao
import com.anpfuel.data.local.entity.ForumCacheEntity
import com.anpfuel.data.local.forum.ForumCacheCodec
import com.anpfuel.data.local.forum.ForumCacheRule
import com.anpfuel.domain.forum.ForumCategoriesResult
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumFeedPage
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.forum.ForumPostKind
import com.anpfuel.domain.repository.ForumGateway
import kotlinx.coroutines.CancellationException

/**
 * Forum repository implementing offline cache and fallback (FOR-07 / B-BR-FOR-01..07).
 *
 * Implements:
 * - 24-hour expiration (stale entries evicted and not returned).
 * - 2 MiB storage cap across the forum cache table.
 * - 5-scope limit on cached feed snapshots.
 * - Refresh replacing cached snapshot.
 * - Live 404 invalidation (removes post detail from cache immediately).
 * - Cross-origin cache separation.
 */
class ForumRepository(
    private val client: ForumGateway,
    private val dao: ForumCacheDao,
    baseUrl: String,
    private val clock: () -> Long = System::currentTimeMillis,
) : ForumGateway {

    private val origin = ForumCacheRule.normalizeOrigin(baseUrl)

    override suspend fun getCategories(): ForumCategoriesResult {
        val key = ForumCacheRule.categoriesKey()
        return try {
            val result = client.getCategories()
            store(key, ForumCacheRule.KIND_CATEGORIES, ForumCacheCodec.encodeCategories(result))
            result
        } catch (e: CancellationException) {
            throw e
        } catch (e: ForumException.Disabled) {
            throw e
        } catch (e: ForumException.Offline) {
            loadCachedCategories(key) ?: throw e
        }
    }

    override suspend fun getFeed(
        category: ForumCategory?,
        kind: ForumPostKind?,
        limit: Int,
        cursor: String?,
    ): ForumFeedPage {
        val scopeKey = ForumCacheRule.feedKey(category, kind)
        if (!cursor.isNullOrBlank()) {
            // Deeper cursor pages are fetched directly from network and not cached
            return client.getFeed(category, kind, limit, cursor)
        }

        return try {
            val page = client.getFeed(category, kind, limit, null)
            store(scopeKey, ForumCacheRule.KIND_FEED, ForumCacheCodec.encodeFeed(page))
            dao.trimFeedScopes(origin, ForumCacheRule.MAX_FEED_SCOPES)
            page
        } catch (e: CancellationException) {
            throw e
        } catch (e: ForumException.Disabled) {
            throw e
        } catch (e: ForumException.Offline) {
            loadCachedFeed(scopeKey) ?: throw e
        }
    }

    override suspend fun getPostDetail(postId: String): ForumPostDetail {
        val detailKey = ForumCacheRule.detailKey(postId)
        return try {
            val detail = client.getPostDetail(postId)
            store(detailKey, ForumCacheRule.KIND_DETAIL, ForumCacheCodec.encodeDetail(detail))
            detail
        } catch (e: CancellationException) {
            throw e
        } catch (e: ForumException.NotFound) {
            // Live 404 invalidation: remove post from cache immediately
            dao.deleteByKey(origin, detailKey)
            throw e
        } catch (e: ForumException.Disabled) {
            throw e
        } catch (e: ForumException.Offline) {
            loadCachedDetail(detailKey) ?: throw e
        }
    }

    private suspend fun store(key: String, kind: String, payload: String) {
        if (payload.length > ForumCacheRule.MAX_PAYLOAD_CHARS) return
        val bytes = payload.toByteArray(Charsets.UTF_8).size.toLong()
        dao.upsert(
            ForumCacheEntity(
                origin = origin,
                key = key,
                kind = kind,
                payloadJson = payload,
                fetchedAtMillis = clock(),
                payloadBytes = bytes,
            ),
        )
        prune()
    }

    private suspend fun prune() {
        // 1. Delete rows older than 24h
        dao.deleteOlderThan(clock() - ForumCacheRule.MAX_CACHE_AGE_MILLIS)
        // 2. Enforce 2 MiB byte budget
        while (dao.totalBytes(origin) > ForumCacheRule.MAX_TOTAL_BYTES) {
            dao.deleteOldest(origin, 5)
        }
    }

    private suspend fun loadCachedCategories(key: String): ForumCategoriesResult? {
        val entity = dao.findByKey(origin, key) ?: return null
        if (isExpired(entity)) return null
        return try {
            ForumCacheCodec.decodeCategories(entity.payloadJson)
        } catch (_: Exception) {
            null
        }
    }

    private suspend fun loadCachedFeed(key: String): ForumFeedPage? {
        val entity = dao.findByKey(origin, key) ?: return null
        if (isExpired(entity)) return null
        return try {
            val page = ForumCacheCodec.decodeFeed(entity.payloadJson)
            // Offline feeds do not support deeper pagination
            page.copy(nextCursor = null)
        } catch (_: Exception) {
            null
        }
    }

    private suspend fun loadCachedDetail(key: String): ForumPostDetail? {
        val entity = dao.findByKey(origin, key) ?: return null
        if (isExpired(entity)) return null
        return try {
            ForumCacheCodec.decodeDetail(entity.payloadJson)
        } catch (_: Exception) {
            null
        }
    }

    private fun isExpired(entity: ForumCacheEntity): Boolean {
        return (clock() - entity.fetchedAtMillis) > ForumCacheRule.MAX_CACHE_AGE_MILLIS
    }
}
