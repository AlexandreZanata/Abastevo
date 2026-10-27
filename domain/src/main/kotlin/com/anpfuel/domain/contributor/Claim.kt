package com.anpfuel.domain.contributor

/**
 * B-BR-CTB contributor claim self-service (CTB-02/03/04/07).
 *
 * The server is authoritative for quotas, states and proofs; these types
 * carry validated user input and server-derived read models only.
 */
enum class ClaimState {
    PENDING,
    APPROVED,
    REJECTED,
    WITHDRAWN,
    EXPIRED,
    UNKNOWN,
    ;

    companion object {
        fun parse(raw: String): ClaimState = entries.firstOrNull { it.name == raw } ?: UNKNOWN
    }
}

enum class ClaimProof {
    UNVERIFIED,
    VERIFIED,
    UNKNOWN,
    ;

    companion object {
        fun parse(raw: String): ClaimProof = entries.firstOrNull { it.name == raw } ?: UNKNOWN
    }
}

data class Claim(
    val id: String,
    val githubLogin: String,
    val state: ClaimState,
    val proof: ClaimProof,
    val gistId: String?,
    val override: Boolean,
    val createdAt: String,
    val expiresAt: String,
    /** One-time code; the server returns it on own reads while PENDING only. */
    val code: String? = null,
) {
    val isOpen: Boolean get() = state == ClaimState.PENDING
}

data class IssuedClaim(
    val claimId: String,
    val code: String,
)

/**
 * Client-side GitHub login check mirroring CTB-03 (server decides).
 * 1–39 chars, alphanumerics or hyphen, no leading/trailing/double hyphen.
 */
fun isValidGithubLogin(raw: String): Boolean {
    val login = raw.trim()
    if (login.isEmpty() || login.length > 39) return false
    if (login.startsWith('-') || login.endsWith('-') || "--" in login) return false
    return login.all { it.isLetterOrDigit() || it == '-' }
}

fun isValidGistId(raw: String): Boolean =
    raw.trim().isNotEmpty() && raw.trim().length <= 128 && !raw.trim().any { it.isWhitespace() }
