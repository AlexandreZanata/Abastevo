package com.anpfuel.application.usecase.forum

import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumReportReason
import com.anpfuel.domain.forum.ForumReportReceipt
import com.anpfuel.domain.repository.ForumGateway

class ReportForumTargetUseCase(
    private val gateway: ForumGateway,
) {
    suspend operator fun invoke(
        targetType: String,
        targetId: String,
        reason: ForumReportReason,
        detail: String? = null,
    ): ForumReportReceipt {
        val normalizedType = targetType.trim().uppercase()
        if (normalizedType != "FORUM_POST" && normalizedType != "FORUM_COMMENT") {
            throw ForumException.Invalid("forum.bad-target-type")
        }
        val trimmedTargetId = targetId.trim()
        if (!UUID_REGEX.matches(trimmedTargetId)) {
            throw ForumException.Invalid("forum.bad-id")
        }
        val trimmedDetail = detail?.trim()?.takeIf { it.isNotEmpty() }
        if (trimmedDetail != null && trimmedDetail.length > 500) {
            throw ForumException.Invalid("forum.detail-too-long")
        }
        return gateway.report(normalizedType, trimmedTargetId, reason.wireCode, trimmedDetail)
    }

    companion object {
        private val UUID_REGEX = Regex(
            "^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$",
        )
    }
}
