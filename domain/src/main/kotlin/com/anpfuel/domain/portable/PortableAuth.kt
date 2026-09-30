package com.anpfuel.domain.portable

/**
 * Portable FREE-account auth values shared by Android and iPhone (P13-T05A).
 *
 * Pure Kotlin with zero `java.*` imports so this file moves unchanged into a
 * future `commonMain` source set. Every bound mirrors the frozen backend
 * rules (`backend/internal/modules/account/domain`, P13-T01): the client
 * only HINTS (format checks, expiry math, nonce binding) while the server
 * enforces (TTL, attempts, replay, rotation). Never treat a client check
 * as proof.
 */
object PortableAuth {

    /** Supported external providers; `email` names the access-code channel. */
    const val PROVIDER_GOOGLE: String = "google"
    const val PROVIDER_APPLE: String = "apple"
    const val CHANNEL_EMAIL: String = "email"

    /** Frozen OIDC issuers (backend allowlist, B-BR-A03). */
    const val ISSUER_GOOGLE: String = "https://accounts.google.com"
    const val ISSUER_APPLE: String = "https://appleid.apple.com"

    /** Frozen server-side audience; client flags are untrusted. */
    const val AUDIENCE: String = "anpfuel-backend"

    /** Access codes are six ASCII digits (backend TTL 600s, 5 attempts). */
    const val CODE_DIGITS: Int = 6

    /** Access-token lifetime, seconds (backend SessionAccessTTLSeconds). */
    const val ACCESS_TTL_SECONDS: Long = 900L

    /** Refresh-family absolute ceiling, seconds (30 days). */
    const val REFRESH_ABSOLUTE_SECONDS: Long = 30L * 24L * 3600L

    /** Deep-link callback host for native provider flows. */
    const val CALLBACK_SCHEME: String = "anpfuel"
    const val CALLBACK_HOST: String = "auth"
    const val CALLBACK_PATH: String = "/callback"

    /**
     * One authenticated session. Tokens are opaque bearer strings shown
     * once by the server; only their hashes persist server-side. The
     * client keeps them in platform secure storage, never logs.
     */
    data class Session(
        val familyId: String,
        val accountId: String,
        val accessToken: String,
        val refreshToken: String,
        /** Wall-clock expiry of the access token, epoch seconds. */
        val accessExpiresAt: Long,
        /** Wall-clock expiry of the refresh family, epoch seconds. */
        val absoluteExpiresAt: Long,
    )

    /**
     * One provider-login attempt binding the request to its proof. The
     * nonce is minted per attempt and consumed exactly once: a callback
     * whose nonce differs from the attempt refuses as mismatch and is
     * never sent (anti-substitution).
     */
    data class LoginAttempt(
        val provider: String,
        val nonce: String,
        val state: String,
    )

    /**
     * Parsed provider callback: the proof plus the anti-CSRF echo. Parse
     * never trusts: [bind] still checks provider, nonce and state against
     * the originating [LoginAttempt].
     */
    data class ProviderCallback(
        val provider: String,
        val idToken: String,
        val nonce: String,
        val state: String,
    )

    /**
     * Client-transport condition (no connectivity, timeout, unparsable
     * server reply). Never a backend verdict; the UI retries these while
     * auth refusals surface their own message (P13-T05B).
     */
    const val UNAVAILABLE: String = "unavailable"

    /** Stable backend verdicts surfaced for UI mapping (never parsed). */
    object Verdict {
        const val OK: String = "ok"
        const val CODE_UNKNOWN: String = "code-unknown"
        const val CODE_CONSUMED: String = "code-consumed"
        const val CODE_EXPIRED: String = "code-expired"
        const val CODE_LOCKED: String = "code-attempts-exhausted"
        const val SESSION_REUSE: String = "session-reuse-revoked"
        const val SESSION_REVOKED: String = "session-revoked"
        const val SESSION_EXPIRED: String = "session-expired"
        const val ACCOUNT_SUSPENDED: String = "account-suspended"
        const val ACCOUNT_DELETED: String = "account-deleted"
    }

    /** True for `google` or `apple`; anything else is malformed. */
    fun isProvider(value: String): Boolean =
        value == PROVIDER_GOOGLE || value == PROVIDER_APPLE

    /** Issuer allowlist lookup; unknown providers map to empty. */
    fun issuerOf(provider: String): String = when (provider) {
        PROVIDER_GOOGLE -> ISSUER_GOOGLE
        PROVIDER_APPLE -> ISSUER_APPLE
        else -> ""
    }

    /** Client-side code shape only: six ASCII digits, nothing more. */
    fun isCodeShape(code: String): Boolean {
        if (code.length != CODE_DIGITS) return false
        for (c in code) {
            if (c < '0' || c > '9') return false
        }
        return true
    }

    /** True when the access token is still live at [nowEpochSeconds]. */
    fun isAccessLive(session: Session, nowEpochSeconds: Long): Boolean =
        nowEpochSeconds < session.accessExpiresAt &&
            nowEpochSeconds < session.absoluteExpiresAt

    /** True when the refresh family may still rotate at [nowEpochSeconds]. */
    fun isRefreshLive(session: Session, nowEpochSeconds: Long): Boolean =
        nowEpochSeconds < session.absoluteExpiresAt

    /**
     * Binds a callback to its attempt. Returns the callback on exact
     * provider/nonce/state match, null otherwise (substitution, replay
     * across attempts, or cross-provider token swap all refuse).
     */
    fun bind(attempt: LoginAttempt, callback: ProviderCallback): ProviderCallback? {
        if (!isProvider(callback.provider)) return null
        if (callback.idToken.isEmpty() || callback.nonce.isEmpty() || callback.state.isEmpty()) return null
        if (callback.provider != attempt.provider) return null
        if (callback.nonce != attempt.nonce) return null
        if (callback.state != attempt.state) return null
        return callback
    }

    /**
     * Parses `anpfuel://auth/callback?provider=..&id_token=..&nonce=..&state=..`.
     * Returns null for wrong scheme/host/path, missing keys, blank values
     * or duplicated keys (duplicates fail: ambiguous callbacks refuse).
     */
    fun parseCallback(url: String): ProviderCallback? {
        val schemeSplit = url.split("://", limit = 2)
        if (schemeSplit.size != 2) return null
        if (schemeSplit[0] != CALLBACK_SCHEME) return null
        val rest = schemeSplit[1]
        val pathSplit = rest.split("?", limit = 2)
        if (pathSplit.size != 2) return null
        if (pathSplit[0] != CALLBACK_HOST + CALLBACK_PATH) return null
        val params = linkedMapOf<String, String>()
        for (pair in pathSplit[1].split("&")) {
            val kv = pair.split("=", limit = 2)
            if (kv.size != 2 || kv[0].isEmpty() || kv[1].isEmpty()) return null
            if (params.containsKey(kv[0])) return null
            params[kv[0]] = kv[1]
        }
        val provider = params["provider"] ?: return null
        val idToken = params["id_token"] ?: return null
        val nonce = params["nonce"] ?: return null
        val state = params["state"] ?: return null
        return ProviderCallback(provider, idToken, nonce, state)
    }

    /** Callback URL this attempt expects (native layer opens/monitors it). */
    fun callbackUrl(): String = "$CALLBACK_SCHEME://$CALLBACK_HOST$CALLBACK_PATH"
}
