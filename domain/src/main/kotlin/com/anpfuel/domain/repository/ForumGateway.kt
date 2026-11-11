package com.anpfuel.domain.repository

import com.anpfuel.domain.forum.ForumCategoriesResult
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumComment
import com.anpfuel.domain.forum.ForumCommentReceipt
import com.anpfuel.domain.forum.ForumCommentsPage
import com.anpfuel.domain.forum.ForumFeedFilters
import com.anpfuel.domain.forum.ForumFeedPage
import com.anpfuel.domain.forum.ForumOwnReactions
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.forum.ForumPostKind
import com.anpfuel.domain.forum.ForumReaction
import com.anpfuel.domain.forum.ForumReactionResult
import com.anpfuel.domain.forum.ForumReactionTarget
import com.anpfuel.domain.forum.ForumRegion
import com.anpfuel.domain.forum.ForumRepliesPage
import com.anpfuel.domain.forum.ForumReplySort
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
        filters: ForumFeedFilters = ForumFeedFilters(),
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

    /** IDs of the signed-in account's own live comments on [postId]. */
    suspend fun getOwnCommentIds(postId: String): Set<String> = throw UnsupportedOperationException()

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

    /** Ranked top-level replies with each one's best answer preview (B-BR-FOR-18). */
    suspend fun getRankedComments(
        postId: String,
        sort: ForumReplySort,
        limit: Int,
        cursor: String?,
    ): ForumCommentsPage = throw UnsupportedOperationException()

    /** Sets the signed caller's reaction; NONE clears it (B-BR-FOR-17). */
    suspend fun setReaction(
        target: ForumReactionTarget,
        targetId: String,
        reaction: ForumReaction,
        operationId: String,
    ): ForumReactionResult = throw UnsupportedOperationException()

    /** The signed caller's reactions in one conversation. */
    suspend fun getOwnReactions(postId: String): ForumOwnReactions = throw UnsupportedOperationException()
}
