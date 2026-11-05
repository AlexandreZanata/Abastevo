package com.anpfuel.domain.forum

import java.net.URI

/**
 * Safe link verification rules for external source links (B-BR-FOR-02 / ADR-019).
 *
 * Strict security constraints:
 * - HTTPS only (cleartext HTTP, file:, content:, javascript: strictly refused).
 * - Must have a valid, non-empty host with at least one dot (domain name).
 * - Must NOT contain user-info (e.g. https://user:pass@host/ is forbidden).
 * - Port must be default (443) or unspecified (-1).
 * - Length must be capped (<= 2048 characters).
 */
object ForumSafeLinks {

    private const val MAX_URL_LENGTH = 2048

    fun isAllowedUrl(raw: String?): Boolean {
        if (raw.isNullOrBlank() || raw.length > MAX_URL_LENGTH) return false
        val trimmed = raw.trim()
        val uri = try {
            URI(trimmed)
        } catch (_: Exception) {
            return false
        }

        if (uri.scheme?.lowercase() != "https") return false
        val host = uri.host
        if (host.isNullOrBlank() || !host.contains('.')) return false
        if (uri.userInfo != null) return false
        if (uri.port != -1 && uri.port != 443) return false

        return true
    }

    fun requireSafeHttpsUrl(raw: String): String {
        if (!isAllowedUrl(raw)) {
            throw ForumException.SecurityViolation("forum.insecure-link")
        }
        return raw.trim()
    }
}
