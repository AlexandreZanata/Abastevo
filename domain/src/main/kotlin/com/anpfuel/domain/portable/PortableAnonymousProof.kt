package com.anpfuel.domain.portable

/**
 * P10-T03 portable anonymous proof-of-possession values.
 *
 * Pure Kotlin with zero `java.*` imports so this file moves unchanged into
 * a future `commonMain` source set. Every bound mirrors the frozen backend
 * profile (`docs/security/identity-profile.md`, P03-T01): the client only
 * builds and shape-checks; the server enforces (TTL, replay, key binding).
 * Never treat a client check as proof.
 *
 * Covered lines (exact order, ASCII, joined by single `\n`):
 * `@method`, `@authority`, `@path`, `@query`, then only when a body is
 * present `content-type` + `content-digest`, then `created`, `expires`,
 * `keyid`, `nonce`. Any other set fails closed server-side.
 */
object PortableAnonymousProof {

    const val PURPOSE_REGISTER: String = "REGISTER"
    const val PURPOSE_SIGN: String = "SIGN"

    /** Challenge freshness window, seconds (backend ChallengeTTL). */
    const val CHALLENGE_TTL_SECONDS: Long = 300L

    /** Identity JSON bodies cap at 64 KiB server-side. */
    const val MAX_BODY_BYTES: Int = 65536

    /** Honest loss notice: reinstall wipes the Keystore key, no recovery. */
    const val LOST_KEY_NOTICE: String =
        "Reinstalling or clearing app data deletes this device key. " +
            "The anonymous identity cannot be recovered without the old key."

    /** Builds the exact signature base lines in frozen order. */
    fun buildBaseLines(
        method: String,
        authority: String,
        path: String,
        query: String,
        contentType: String?,
        contentDigest: String?,
        created: Long,
        expires: Long,
        keyId: String,
        nonce: String,
    ): List<String> {
        val lines = ArrayList<String>(10)
        lines.add("\"@method\": $method")
        lines.add("\"@authority\": $authority")
        lines.add("\"@path\": $path")
        lines.add("\"@query\": $query")
        if (contentType != null && contentDigest != null) {
            lines.add("\"content-type\": $contentType")
            lines.add("\"content-digest\": $contentDigest")
        }
        lines.add("\"created\": $created")
        lines.add("\"expires\": $expires")
        lines.add("\"keyid\": \"$keyId\"")
        lines.add("\"nonce\": \"$nonce\"")
        return lines
    }

    /** Joins base lines with single `\n`, no trailing newline. */
    fun buildBase(lines: List<String>): String = lines.joinToString("\n")

    /**
     * Shape-checks the covered set: exactly 8 (bodyless) or 10 (body)
     * lines in frozen order, no missing/extra/reordered/duplicated lines.
     */
    fun isLinesShapeValid(lines: List<String>): Boolean {
        if (lines.size != 8 && lines.size != 10) return false
        val want = ArrayList<String>(10)
        want.add("\"@method\":")
        want.add("\"@authority\":")
        want.add("\"@path\":")
        want.add("\"@query\":")
        if (lines.size == 10) {
            want.add("\"content-type\":")
            want.add("\"content-digest\":")
        }
        want.add("\"created\":")
        want.add("\"expires\":")
        want.add("\"keyid\":")
        want.add("\"nonce\":")
        for (i in lines.indices) {
            if (!lines[i].startsWith(want[i])) return false
        }
        return true
    }

    /** Accepts exactly REGISTER or SIGN (case-insensitive, trimmed). */
    fun parsePurpose(value: String): String? {
        return when (value.trim().uppercase()) {
            PURPOSE_REGISTER -> PURPOSE_REGISTER
            PURPOSE_SIGN -> PURPOSE_SIGN
            else -> null
        }
    }

    /** Shape of `fp:` plus 64 lowercase hex chars (no hashing here). */
    fun isFingerprintShape(value: String): Boolean {
        val fp = value.trim()
        if (!fp.startsWith("fp:") || fp.length != 67) return false
        for (c in fp.substring(3)) {
            if (!((c in '0'..'9') || (c in 'a'..'f'))) return false
        }
        return true
    }

    /**
     * Splits `<challenge-id>.<client-nonce>`; both sides must be non-empty
     * printable matter without whitespace. Returns null when malformed.
     */
    fun splitNonce(nonce: String): Pair<String, String>? {
        val dot = nonce.indexOf('.')
        if (dot <= 0 || dot >= nonce.length - 1) return null
        for (c in nonce) {
            if (c <= ' ' || c > '~') return null
        }
        return Pair(nonce.substring(0, dot), nonce.substring(dot + 1))
    }

    /**
     * Window check mirroring the server: `expires > created`,
     * `expires - created <= 300`, `now < expires`,
     * `created <= now + 300`. Zero skew beyond the window.
     */
    fun isWindowValid(created: Long, expires: Long, nowEpochSeconds: Long): Boolean {
        if (created < 0 || expires <= created) return false
        if (expires - created > CHALLENGE_TTL_SECONDS) return false
        if (nowEpochSeconds >= expires) return false
        if (created > nowEpochSeconds + CHALLENGE_TTL_SECONDS) return false
        return true
    }

    /**
     * Canonical rotation intent bytes (UTF-8, no whitespace, exact field
     * order): both old and new proofs sign these same bytes so neither key
     * alone can move the identity.
     */
    fun rotationIntentJson(
        newJwkX: String,
        newJwkY: String,
        oldChallengeId: String,
        newChallengeId: String,
    ): String =
        "{\"new_jwk\":{\"kty\":\"EC\",\"crv\":\"P-256\",\"x\":\"" + newJwkX +
            "\",\"y\":\"" + newJwkY + "\"},\"old_challenge_id\":\"" + oldChallengeId +
            "\",\"new_challenge_id\":\"" + newChallengeId + "\"}"

    /** RFC 7638 canonical thumbprint input (exact member order, no spaces). */
    fun thumbprintInput(jwkX: String, jwkY: String): String =
        "{\"crv\":\"P-256\",\"kty\":\"EC\",\"x\":\"" + jwkX + "\",\"y\":\"" + jwkY + "\"}"
}
