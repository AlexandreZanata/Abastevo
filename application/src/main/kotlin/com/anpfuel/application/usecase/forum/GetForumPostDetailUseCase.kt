package com.anpfuel.application.usecase.forum

import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.repository.ForumGateway

/**
 * Reads full detail of a published forum post (B-BR-FOR-01..07 / BUC-FOR-02).
 *
 * Validates canonical UUID format before dispatching network/cache query.
 */
class GetForumPostDetailUseCase(
    private val gateway: ForumGateway,
) {
    suspend operator fun invoke(postId: String): ForumPostDetail {
        val trimmed = postId.trim()
        if (!UUID_REGEX.matches(trimmed)) {
            throw ForumException.Invalid("forum.bad-id")
        }
        return gateway.getPostDetail(trimmed)
    }

    companion object {
        private val UUID_REGEX = Regex(
            "^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$",
        )
    }
}
