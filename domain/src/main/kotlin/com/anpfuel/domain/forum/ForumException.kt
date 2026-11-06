package com.anpfuel.domain.forum

/**
 * Typed domain errors for Forum operations.
 */
sealed class ForumException(val code: String, message: String = code) : Exception(message) {
    class Invalid(code: String) : ForumException(code)
    class NotFound(code: String) : ForumException(code)
    class Disabled(code: String) : ForumException(code)
    class SecurityViolation(code: String) : ForumException(code)
    class Unavailable(code: String) : ForumException(code)
    class Offline(code: String) : ForumException(code)
    class AuthRequired(code: String) : ForumException(code)
    class Conflict(code: String) : ForumException(code)
    class RateLimited(code: String, val retryAfterSeconds: Long? = null) : ForumException(code)
}
