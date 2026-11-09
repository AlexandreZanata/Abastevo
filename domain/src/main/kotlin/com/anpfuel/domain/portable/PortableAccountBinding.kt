package com.anpfuel.domain.portable

/**
 * Account-to-device binding ceremony values (P13-T04C, B-BR-A04).
 *
 * Pure Kotlin with zero `java.*` imports. The statement and proof layout
 * mirror the backend key prover (`account/adapters/keyprover`): the device
 * key signs SHA-256 of [statement], and the proof travels as
 * `<fingerprint>.<expiry>.<base64url R || S>`. The server enforces the
 * window and signature; these helpers only build the exact bytes.
 */
object PortableAccountBinding {

    /** Server freshness bound: expiry must lie within this many seconds of now. */
    const val PROOF_TTL_SECONDS: Long = 300

    /** Requested proof lifetime, inside the server window with clock-skew slack. */
    const val PROOF_LIFETIME_SECONDS: Long = 120

    /** Binding statement signed by the contributor device key. */
    fun statement(accountId: String, contributorId: String, expiry: Long): String =
        "anpfuel-bind.v1\n$accountId\n$contributorId\n$expiry"

    /** Wire proof; null when any part is outside the accepted shape. */
    fun proof(fingerprint: String, expiry: Long, signatureB64Url: String): String? {
        if (!PortableAnonymousProof.isFingerprintShape(fingerprint)) return null
        if (expiry <= 0 || signatureB64Url.isEmpty()) return null
        if (!signatureB64Url.all { it in 'A'..'Z' || it in 'a'..'z' || it in '0'..'9' || it == '-' || it == '_' }) return null
        return "$fingerprint.$expiry.$signatureB64Url"
    }
}
