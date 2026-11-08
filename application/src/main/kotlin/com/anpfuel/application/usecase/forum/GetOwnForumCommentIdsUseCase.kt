package com.anpfuel.application.usecase.forum

import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.repository.ForumGateway

class GetOwnForumCommentIdsUseCase(
    private val gateway: ForumGateway,
) {
    suspend operator fun invoke(postId: String): Set<String> {
        val trimmedPostId = postId.trim()
        if (!SubmitForumTopicUseCase.UUID_REGEX.matches(trimmedPostId)) {
            throw ForumException.Invalid("forum.bad-id")
        }
        return gateway.getOwnCommentIds(trimmedPostId)
    }
}
