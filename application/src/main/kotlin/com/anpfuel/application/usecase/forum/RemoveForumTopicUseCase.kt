package com.anpfuel.application.usecase.forum

import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumTopicReceipt
import com.anpfuel.domain.repository.ForumGateway
import java.util.UUID

class RemoveForumTopicUseCase(
    private val gateway: ForumGateway,
) {
    suspend operator fun invoke(
        topicId: String,
        operationId: String = UUID.randomUUID().toString(),
    ): ForumTopicReceipt {
        val trimmedTopicId = topicId.trim()
        if (!SubmitForumTopicUseCase.UUID_REGEX.matches(trimmedTopicId)) {
            throw ForumException.Invalid("forum.bad-id")
        }
        if (!SubmitForumTopicUseCase.UUID_REGEX.matches(operationId.trim())) {
            throw ForumException.Invalid("forum.bad-operation-id")
        }
        return gateway.removeTopic(trimmedTopicId, operationId.trim())
    }
}
