package com.anpfuel.domain.repository

import com.anpfuel.domain.forum.ForumCategoriesResult
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumFeedPage
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.forum.ForumPostKind

/**
 * Contract for Forum Abastevo remote and cache access.
 */
interface ForumGateway {
    suspend fun getCategories(): ForumCategoriesResult
    suspend fun getFeed(
        category: ForumCategory? = null,
        kind: ForumPostKind? = null,
        limit: Int = 20,
        cursor: String? = null,
    ): ForumFeedPage
    suspend fun getPostDetail(postId: String): ForumPostDetail
}
