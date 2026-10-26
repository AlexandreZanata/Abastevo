package com.anpfuel.data.remote

import java.net.URI

/**
 * P34-T02 explicit backend origin/environment (B-BR-011 privacy).
 *
 * Single source of truth for auth, anonymous identity, community, feedback
 * and contribution clients. [STAGING] selects `https://teste.abastevo.com.br`;
 * [PREVIEW] is the RFC 2606 non-resolving placeholder. The release target
 * stays explicit and separately certified: it is never inferred from staging
 * and must not reuse the test origin without its own certification.
 *
 * Origins carry no secret, token, key or precise location by construction.
 * Route joining never produces a double `/v1`; origins containing a `/v1`
 * path are refused so clients cannot silently build `/v1/v1/...` URLs.
 */
enum class ApiEnvironment(val origin: String, private val localProof: Boolean = false) {
    PREVIEW("https://api.anpfuel.example.invalid"),
    STAGING("https://teste.abastevo.com.br"),
    // Debug-build loopback proof only. Release network security forbids
    // cleartext, so an accidental release selection still fails closed.
    LOCAL_PROOF("http://127.0.0.1:18080", localProof = true);

    init {
        if (localProof) validateLocalProofOrigin(origin) else validateOrigin(origin)
    }

    fun join(path: String): String {
        require(path.startsWith("/")) { "route path must start with /: $path" }
        return origin.trimEnd('/') + path
    }

    companion object {
        fun requireValidOrigin(value: String): String = validateOrigin(value)
    }
}

private fun validateLocalProofOrigin(value: String): String {
    require(value == "http://127.0.0.1:18080") { "local proof origin must be the loopback proof address: $value" }
    return value
}

private fun validateOrigin(value: String): String {
    require(value.isNotBlank()) { "origin is blank" }
    val uri = try {
        URI(value)
    } catch (_: Exception) {
        throw IllegalArgumentException("origin malformed: $value")
    }
    require(uri.scheme == "https") { "origin must be https: $value" }
    require(!uri.host.isNullOrBlank()) { "origin host missing: $value" }
    require(uri.userInfo == null) { "origin must not carry user info: $value" }
    require(uri.query == null && uri.fragment == null) { "origin must not carry query/fragment: $value" }
    val path = uri.path.orEmpty()
    require(path.isEmpty() || path == "/") { "origin must not carry a path (no /v1): $value" }
    return value
}
