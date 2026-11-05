package com.anpfuel.application.usecase.forum

import com.anpfuel.domain.forum.ForumSafeLinks

/**
 * Validates external source attribution URLs before rendering or opening (B-BR-FOR-02 / ADR-019).
 *
 * Enforces HTTPS-only, rejects credentials in URLs and non-standard ports.
 */
class ValidateForumSafeLinkUseCase {

    operator fun invoke(rawUrl: String?): Boolean {
        return ForumSafeLinks.isAllowedUrl(rawUrl)
    }

    fun requireSafe(rawUrl: String): String {
        return ForumSafeLinks.requireSafeHttpsUrl(rawUrl)
    }
}
