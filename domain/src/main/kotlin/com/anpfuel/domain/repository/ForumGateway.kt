package com.anpfuel.domain.repository

import com.anpfuel.domain.forum.ForumCategoriesResult
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumComment
import com.anpfuel.domain.forum.ForumCommentReceipt
import com.anpfuel.domain.forum.ForumCommentsPage
import com.anpfuel.domain.forum.ForumFeedPage
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.forum.ForumPostKind
import com.anpfuel.domain.forum.ForumRegion
import com.anpfuel.domain.forum.ForumRepliesPage
import com.anpfuel.domain.forum.ForumReportReceipt
import com.anpfuel.domain.forum.ForumTopicReceipt

/**
 * Contract for Forum Abastevo remote and cache access.
 */
interface ForumGateway {
    suspend fun getCategories(): ForumCategoriesResult = throw UnsupportedOperationException()
    suspend fun getFeed(
        category: ForumCategory? = null,
        kind: ForumPostKind? = null,
        limit: Int = 20,
        cursor: String? = null,
    ): ForumFeedPage = throw UnsupportedOperationException()
    suspend fun getPostDetail(postId: String): ForumPostDetail = throw UnsupportedOperationException()

    suspend fun getComments(
        postId: String,
        limit: Int = 20,
        cursor: String? = null,
    ): ForumCommentsPage = throw UnsupportedOperationException()

    suspend fun getReplies(
        commentId: String,
        limit: Int = 20,
        cursor: String? = null,
    ): ForumRepliesPage = throw UnsupportedOperationException()

    suspend fun createComment(
        postId: String,
        parentId: String? = null,
        bodyText: String,
        operationId: String,
    ): ForumCommentReceipt = throw UnsupportedOperationException()

    suspend fun editComment(
        commentId: String,
        expectedRevision: Long,
        bodyText: String,
        operationId: String,
    ): ForumCommentReceipt = throw UnsupportedOperationException()

    suspend fun removeComment(
        commentId: String,
        expectedRevision: Long,
        operationId: String,
    ): ForumCommentReceipt = throw UnsupportedOperationException()

    suspend fun getOwnerComment(commentId: String): ForumCommentReceipt = throw UnsupportedOperationException()

    suspend fun report(
        targetType: String,
        targetId: String,
        reason: String,
        detail: String? = null,
    ): ForumReportReceipt = throw UnsupportedOperationException()

    suspend fun createTopic(
        title: String,
        bodyText: String,
        category: ForumCategory,
        operationId: String,
        region: ForumRegion? = null,
        fuelProduct: String? = null,
        stationId: String? = null,
        publicAlias: String? = null,
    ): ForumTopicReceipt = throw UnsupportedOperationException()

    suspend fun editTopic(
        topicId: String,
        expectedRevision: Long,
        title: String,
        bodyText: String,
        category: ForumCategory,
        operationId: String,
        region: ForumRegion? = null,
        fuelProduct: String? = null,
        stationId: String? = null,
        publicAlias: String? = null,
    ): ForumTopicReceipt = throw UnsupportedOperationException()

    suspend fun removeTopic(
        topicId: String,
        operationId: String,
    ): ForumTopicReceipt = throw UnsupportedOperationException()

    suspend fun getOwnerTopic(topicId: String): ForumTopicReceipt = throw UnsupportedOperationException()
}
