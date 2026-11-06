package com.anpfuel.application.usecase.forum

import com.anpfuel.domain.forum.ForumCommentsPage
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.repository.ForumGateway

class GetForumCommentsUseCase(
    private val gateway: ForumGateway,
) {
    suspend operator fun invoke(
        postId: String,
        limit: Int = 20,
        cursor: String? = null,
    ): ForumCommentsPage {
        val trimmed = postId.trim()
        if (!UUID_REGEX.matches(trimmed)) {
            throw ForumException.Invalid("forum.bad-id")
        }
        val clampedLimit = limit.coerceIn(1, 50)
        return gateway.getComments(trimmed, clampedLimit, cursor?.takeIf { it.isNotBlank() })
    }

    companion object {
        private val UUID_REGEX = Regex(
            "^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$",
        )
    }
}
