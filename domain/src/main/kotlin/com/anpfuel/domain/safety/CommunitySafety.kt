package com.anpfuel.domain.safety

import com.anpfuel.domain.portable.PortableText

/**
 * Public UGC surfaces a user report or block may name (B-BR-SAFE-01,
 * B-BR-SAFE-02). The value is the wire `content_kind`.
 */
enum class SafetyContentKind(val wire: String) {
    /** Forum topics only (post kind `TOPIC`); editorial, news and bulletin posts are out of scope. */
    FORUM_POST("FORUM_POST"),

    /** Forum comments and replies. */
    FORUM_COMMENT("FORUM_COMMENT"),

    /** Station comments and replies. */
    STATION_COMMENT("STATION_COMMENT"),
    ;

    companion object {
        /** Null for an unknown or missing value, never a guess. */
        fun fromWire(value: String?): SafetyContentKind? = entries.firstOrNull { it.wire == value?.trim() }
    }
}

/**
 * The only way the app names a user: a public content reference whose
 * author the server derives (B-BR-SAFE-02). Ids compare case-insensitively,
 * like the UUIDs they are, so a reference echoed by the server matches the
 * one the app built.
 */
class SafetyContentRef(val kind: SafetyContentKind, id: String) {

    /** Trimmed, lowercase content UUID. */
    val id: String = id.trim().lowercase()

    /** True when [id] is a canonical UUID the server can accept. */
    val isWellFormed: Boolean
        get() = UUID_PATTERN.matches(id)

    override fun equals(other: Any?): Boolean =
        other is SafetyContentRef && other.kind == kind && other.id == id

    override fun hashCode(): Int = 31 * kind.hashCode() + id.hashCode()

    override fun toString(): String = "SafetyContentRef(${kind.wire}, $id)"

    private companion object {
        val UUID_PATTERN = Regex("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")
    }
}

/** Reasons for reporting a user (B-BR-SAFE-04); the value is the wire code. */
enum class UserReportReason(val wire: String) {
    SPAM("SPAM"),
    HARASSMENT("HARASSMENT"),
    HATE("HATE"),
    IMPERSONATION("IMPERSONATION"),
    ILLEGAL("ILLEGAL"),
    OTHER("OTHER"),
    ;

    companion object {
        /** Unknown codes read as [OTHER]; the receipt still confirms the report. */
        fun fromWire(value: String?): UserReportReason = entries.firstOrNull { it.wire == value?.trim() } ?: OTHER
    }
}

/**
 * Optional private detail of a user report (B-BR-SAFE-04): plain text,
 * trimmed with line breaks normalized, at most [MAX_SCALARS] Unicode scalar
 * values counted like the server does.
 */
object UserReportDetail {
    const val MAX_SCALARS: Int = 200

    /** The text to send, or null when the detail is blank. */
    fun normalize(raw: String?): String? {
        if (raw == null) return null
        return PortableText.normalize(raw).takeIf { it.isNotEmpty() }
    }

    /** True when the detail is absent or fits the limit after normalization. */
    fun isValid(raw: String?): Boolean {
        val normalized = normalize(raw) ?: return true
        return PortableText.countScalars(normalized) <= MAX_SCALARS
    }

    /** Scalars still available for [raw]; negative once over the limit. */
    fun remaining(raw: String?): Int = MAX_SCALARS - (normalize(raw)?.let(PortableText::countScalars) ?: 0)
}

/** Acknowledgment of a user report; a repeat returns the first one with [replayed] (B-BR-SAFE-04). */
data class UserReportReceipt(
    val reportId: String,
    val reason: UserReportReason,
    val createdAt: String,
    val replayed: Boolean,
)

/**
 * One block owned by the signed account (B-BR-SAFE-07). [label] is the public
 * alias seen when the block was made and may be empty; [sourceKind] is null
 * when the server sends a kind this app version does not know.
 */
data class BlockedUser(
    val blockId: String,
    val label: String,
    val sourceKind: SafetyContentKind?,
    val createdAt: String,
)

/** Result of a block; blocking the same account again returns the existing block (B-BR-SAFE-05). */
data class BlockReceipt(
    val blockedUser: BlockedUser,
    val replayed: Boolean,
)

/**
 * Typed refusals of the community safety API. Messages carry only the stable
 * code, never a server body.
 */
sealed class CommunitySafetyException(val code: String) : Exception(code) {
    class BadRequest(code: String = "safety.bad-request") : CommunitySafetyException(code)

    /** The content belongs to the caller (B-BR-SAFE-02). */
    class SelfTarget : CommunitySafetyException("safety.self-target")

    /** No live signed session: the app sends the user to sign in (B-BR-SAFE-03). */
    class AuthRequired : CommunitySafetyException("safety.auth-required")

    /** Missing, hidden or foreign content or block. */
    class NotFound : CommunitySafetyException("safety.not-found")

    /** The account already holds the maximum number of active blocks (B-BR-SAFE-05). */
    class BlockLimit : CommunitySafetyException("safety.block-limit")

    class RateLimited(val retryAfterSeconds: Long?) : CommunitySafetyException("safety.rate-limited")

    class Unavailable(code: String = "safety.unavailable") : CommunitySafetyException(code)

    class Offline : CommunitySafetyException("safety.offline")
}

/** Signed community safety operations (contract: COMMUNITY_SAFETY_CONTRACT wire table). */
interface CommunitySafetyGateway {
    suspend fun reportUser(ref: SafetyContentRef, reason: UserReportReason, detail: String?): UserReportReceipt

    suspend fun blockUser(ref: SafetyContentRef): BlockReceipt

    /** The caller's blocks, newest first, at most 500. */
    suspend fun listBlocks(): List<BlockedUser>

    suspend fun removeBlock(blockId: String)

    /** The subset of 1..100 [refs] written by accounts the caller blocked (B-BR-SAFE-06). */
    suspend fun hiddenContent(refs: List<SafetyContentRef>): Set<SafetyContentRef>
}
