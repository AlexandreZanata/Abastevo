package com.anpfuel.application.usecase.forum

import com.anpfuel.domain.forum.ForumCommentReceipt
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.repository.ForumGateway
import java.util.UUID

class SubmitForumCommentUseCase(
    private val gateway: ForumGateway,
) {
    suspend operator fun invoke(
        postId: String,
        bodyText: String,
        parentId: String? = null,
        operationId: String = UUID.randomUUID().toString(),
    ): ForumCommentReceipt {
        val trimmedPostId = postId.trim()
        if (!UUID_REGEX.matches(trimmedPostId)) {
            throw ForumException.Invalid("forum.bad-id")
        }
        val trimmedParentId = parentId?.trim()?.takeIf { it.isNotEmpty() }
        if (trimmedParentId != null && !UUID_REGEX.matches(trimmedParentId)) {
            throw ForumException.Invalid("forum.bad-parent-id")
        }
        val trimmedBody = bodyText.trim()
        if (trimmedBody.isEmpty()) {
            throw ForumException.Invalid("forum.comment-empty")
        }
        if (trimmedBody.length > 280) {
            throw ForumException.Invalid("forum.comment-too-long")
        }
        return gateway.createComment(trimmedPostId, trimmedParentId, trimmedBody, operationId)
    }

    companion object {
        private val UUID_REGEX = Regex(
            "^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$",
        )
    }
}
