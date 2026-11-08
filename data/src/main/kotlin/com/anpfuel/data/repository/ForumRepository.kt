package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.ForumCacheDao
import com.anpfuel.data.local.entity.ForumCacheEntity
import com.anpfuel.data.local.forum.ForumCacheCodec
import com.anpfuel.data.local.forum.ForumCacheRule
import com.anpfuel.domain.forum.ForumCategoriesResult
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumCommentReceipt
import com.anpfuel.domain.forum.ForumCommentsPage
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumFeedFilters
import com.anpfuel.domain.forum.ForumFeedPage
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.forum.ForumPostKind
import com.anpfuel.domain.forum.ForumRegion
import com.anpfuel.domain.forum.ForumRepliesPage
import com.anpfuel.domain.forum.ForumReportReceipt
import com.anpfuel.domain.forum.ForumTopicReceipt
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
        filters: ForumFeedFilters,
    ): ForumFeedPage {
        val scopeKey = ForumCacheRule.feedKey(category, kind, filters)
        if (!cursor.isNullOrBlank()) {
            // Deeper cursor pages are fetched directly from network and not cached
            return client.getFeed(category, kind, limit, cursor, filters)
        }

        return try {
            val page = client.getFeed(category, kind, limit, null, filters)
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

    override suspend fun getComments(postId: String, limit: Int, cursor: String?): ForumCommentsPage {
        return client.getComments(postId, limit, cursor)
    }

    override suspend fun getReplies(commentId: String, limit: Int, cursor: String?): ForumRepliesPage {
        return client.getReplies(commentId, limit, cursor)
    }

    override suspend fun createComment(
        postId: String,
        parentId: String?,
        bodyText: String,
        operationId: String,
    ): ForumCommentReceipt {
        return client.createComment(postId, parentId, bodyText, operationId)
    }

    override suspend fun editComment(
        commentId: String,
        expectedRevision: Long,
        bodyText: String,
        operationId: String,
    ): ForumCommentReceipt {
        return client.editComment(commentId, expectedRevision, bodyText, operationId)
    }

    override suspend fun removeComment(
        commentId: String,
        expectedRevision: Long,
        operationId: String,
    ): ForumCommentReceipt {
        return client.removeComment(commentId, expectedRevision, operationId)
    }

    override suspend fun getOwnerComment(commentId: String): ForumCommentReceipt {
        return client.getOwnerComment(commentId)
    }

    override suspend fun report(
        targetType: String,
        targetId: String,
        reason: String,
        detail: String?,
    ): ForumReportReceipt {
        return client.report(targetType, targetId, reason, detail)
    }

    override suspend fun createTopic(
        title: String,
        bodyText: String,
        category: ForumCategory,
        operationId: String,
        region: ForumRegion?,
        fuelProduct: String?,
        stationId: String?,
        publicAlias: String?,
    ): ForumTopicReceipt {
        return client.createTopic(title, bodyText, category, operationId, region, fuelProduct, stationId, publicAlias)
    }

    override suspend fun editTopic(
        topicId: String,
        expectedRevision: Long,
        title: String,
        bodyText: String,
        category: ForumCategory,
        operationId: String,
        region: ForumRegion?,
        fuelProduct: String?,
        stationId: String?,
        publicAlias: String?,
    ): ForumTopicReceipt {
        return client.editTopic(topicId, expectedRevision, title, bodyText, category, operationId, region, fuelProduct, stationId, publicAlias)
    }

    override suspend fun removeTopic(
        topicId: String,
        operationId: String,
    ): ForumTopicReceipt {
        return client.removeTopic(topicId, operationId)
    }

    override suspend fun getOwnerTopic(topicId: String): ForumTopicReceipt {
        return client.getOwnerTopic(topicId)
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
