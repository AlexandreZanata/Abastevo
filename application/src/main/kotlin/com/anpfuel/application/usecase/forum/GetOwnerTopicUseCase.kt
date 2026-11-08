package com.anpfuel.application.usecase.forum

import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumTopicReceipt
import com.anpfuel.domain.repository.ForumGateway

class GetOwnerTopicUseCase(
    private val gateway: ForumGateway,
) {
    suspend operator fun invoke(
        topicId: String,
    ): ForumTopicReceipt {
        val trimmedTopicId = topicId.trim()
        if (!SubmitForumTopicUseCase.UUID_REGEX.matches(trimmedTopicId)) {
            throw ForumException.Invalid("forum.bad-id")
        }
        return gateway.getOwnerTopic(trimmedTopicId)
    }
}
