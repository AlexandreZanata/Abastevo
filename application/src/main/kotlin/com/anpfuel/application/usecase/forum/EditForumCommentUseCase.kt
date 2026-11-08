package com.anpfuel.application.usecase.forum

import com.anpfuel.domain.forum.ForumCommentReceipt
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.repository.ForumGateway
import java.util.UUID

class EditForumCommentUseCase(
    private val gateway: ForumGateway,
) {
    suspend operator fun invoke(
        commentId: String,
        expectedRevision: Long,
        bodyText: String,
        operationId: String = UUID.randomUUID().toString(),
    ): ForumCommentReceipt {
        val trimmedCommentId = commentId.trim()
        if (!UUID_REGEX.matches(trimmedCommentId)) {
            throw ForumException.Invalid("forum.bad-id")
        }
        val trimmedBody = bodyText.trim()
        if (trimmedBody.isEmpty()) {
            throw ForumException.Invalid("forum.comment-empty")
        }
        if (trimmedBody.length > 280) {
            throw ForumException.Invalid("forum.comment-too-long")
        }
        return gateway.editComment(trimmedCommentId, expectedRevision, trimmedBody, operationId)
    }

    companion object {
        private val UUID_REGEX = Regex(
            "^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$",
        )
    }
}
