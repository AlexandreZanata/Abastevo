package com.anpfuel.application.usecase.forum

import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumRepliesPage
import com.anpfuel.domain.repository.ForumGateway

class GetForumRepliesUseCase(
    private val gateway: ForumGateway,
) {
    suspend operator fun invoke(
        commentId: String,
        limit: Int = 20,
        cursor: String? = null,
    ): ForumRepliesPage {
        val trimmed = commentId.trim()
        if (!UUID_REGEX.matches(trimmed)) {
            throw ForumException.Invalid("forum.bad-id")
        }
        val clampedLimit = limit.coerceIn(1, 50)
        return gateway.getReplies(trimmed, clampedLimit, cursor?.takeIf { it.isNotBlank() })
    }

    companion object {
        private val UUID_REGEX = Regex(
            "^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$",
        )
    }
}
