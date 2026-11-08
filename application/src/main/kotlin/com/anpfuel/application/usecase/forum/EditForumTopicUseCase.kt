package com.anpfuel.application.usecase.forum

import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumRegion
import com.anpfuel.domain.forum.ForumTopicReceipt
import com.anpfuel.domain.repository.ForumGateway
import java.util.UUID

class EditForumTopicUseCase(
    private val gateway: ForumGateway,
) {
    suspend operator fun invoke(
        topicId: String,
        expectedRevision: Long,
        title: String,
        bodyText: String,
        category: ForumCategory,
        operationId: String = UUID.randomUUID().toString(),
        region: ForumRegion? = null,
        fuelProduct: String? = null,
        stationId: String? = null,
        publicAlias: String? = null,
    ): ForumTopicReceipt {
        val trimmedTopicId = topicId.trim()
        if (!SubmitForumTopicUseCase.UUID_REGEX.matches(trimmedTopicId)) {
            throw ForumException.Invalid("forum.bad-id")
        }

        if (expectedRevision < 1) {
            throw ForumException.Invalid("forum.bad-expected-revision")
        }

        val trimmedTitle = title.trim()
        if (trimmedTitle.isEmpty()) {
            throw ForumException.Invalid("forum.topic-title-empty")
        }
        if (trimmedTitle.length > 120) {
            throw ForumException.Invalid("forum.topic-title-too-long")
        }

        val trimmedBody = bodyText.trim()
        if (trimmedBody.isEmpty()) {
            throw ForumException.Invalid("forum.topic-body-empty")
        }
        if (trimmedBody.length > 4000) {
            throw ForumException.Invalid("forum.topic-body-too-long")
        }

        if (category == ForumCategory.UNKNOWN) {
            throw ForumException.Invalid("forum.topic-category-invalid")
        }

        if (!SubmitForumTopicUseCase.UUID_REGEX.matches(operationId.trim())) {
            throw ForumException.Invalid("forum.bad-operation-id")
        }

        val validatedRegion = SubmitForumTopicUseCase.validateRegion(region)
        val validatedFuel = SubmitForumTopicUseCase.validateFuelProduct(fuelProduct)
        val validatedStationId = SubmitForumTopicUseCase.validateStationId(stationId)
        val trimmedAlias = publicAlias?.trim()?.takeIf { it.isNotEmpty() }

        return gateway.editTopic(
            topicId = trimmedTopicId,
            expectedRevision = expectedRevision,
            title = trimmedTitle,
            bodyText = trimmedBody,
            category = category,
            operationId = operationId.trim(),
            region = validatedRegion,
            fuelProduct = validatedFuel,
            stationId = validatedStationId,
            publicAlias = trimmedAlias,
        )
    }
}
