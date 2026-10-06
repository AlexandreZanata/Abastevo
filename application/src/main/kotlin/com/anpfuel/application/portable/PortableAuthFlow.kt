package com.anpfuel.application.portable

import com.anpfuel.domain.portable.PortableAuth

/**
 * Portable FREE-account login flows shared by Android and iPhone (P13-T05A).
 *
 * Pure Kotlin with zero `java.*`/Android imports so this file moves unchanged
 * into a future `commonMain` source set. The flows orchestrate the frozen
 * backend contract (P13-T01…T04: email codes, rotating sessions, OIDC
 * links, suspension/deletion verdicts); the server always enforces, the
 * client only hints, binds nonces and persists sessions. Tests inject
 * deterministic fakes; no real clock, randomness, storage or I/O lives here.
 *
 * Login methods: email access-code signup/login plus Google/Apple links
 * onto the email-created account (the backend links verified provider
 * subjects; equal emails never merge). Native provider SDKs and secure
 * storage arrive behind [AuthPorts]; device evidence stays deferred to
 * the release horizon per owner decision.
 */
class AuthFlow(private val ports: AuthPorts) {

    /** Local-hint refusal code; never sent, never a backend verdict. */
    companion object {
        const val CLIENT_INVALID: String = "client-invalid"

        /** Backend bodies cap at 8KiB; proofs must fit comfortably inside. */
        const val MAX_PROOF_CHARS: Int = 8192
    }

    /** Opaque login outcome: the session plus whether signup created it. */
    data class Login(
        val session: PortableAuth.Session,
        val created: Boolean,
    )

    /** One-display account key issued at key-account signup. */
    data class KeyIssued(
        val username: String,
        val accountKey: String,
    )

    /** Stored key backup: username plus the only login secret. */
    data class KeyBackup(
        val username: String,
        val accountKey: String,
    )

    /** Rehydrated state after start or process death. */
    sealed interface AuthState {
        data object LoggedOut : AuthState
        data class Active(val session: PortableAuth.Session) : AuthState
        data class NeedsRefresh(val session: PortableAuth.Session) : AuthState
    }

    /** Requests an access code. Unknown addresses answer identically. */
    fun requestEmailCode(email: String): AuthApiResult<Unit> {
        if (!isEmailHint(email)) return AuthApiResult.Err(CLIENT_INVALID)
        return ports.api.requestCode(email.trim())
    }

    /**
     * Redeems one code for a session and persists it. Wrong codes share
     * the server `code-unknown` refusal; suspended accounts refuse
     * without burning the code (server-side, mirrored here by exact
     * verdict passthrough).
     */
    fun consumeEmailCode(email: String, code: String): AuthApiResult<Login> {
        if (!isEmailHint(email) || !PortableAuth.isCodeShape(code)) {
            return AuthApiResult.Err(CLIENT_INVALID)
        }
        return when (val res = ports.api.consumeCode(email.trim(), code)) {
            is AuthApiResult.Err -> res
            is AuthApiResult.Ok -> {
                ports.store.save(res.value.session)
                AuthApiResult.Ok(Login(res.value.session, res.value.created))
            }
        }
    }

    /**
     * Creates an anonymous key account for username and persists the
     * one-display key for backup. Creation alone logs nothing in: the
     * contributor enters with the key via [loginWithKey], so the backup
     * moment cannot be skipped.
     */
    fun createKeyAccount(username: String): AuthApiResult<KeyIssued> {
        val name = username.trim().lowercase()
        if (!isUsernameHint(name)) return AuthApiResult.Err(CLIENT_INVALID)
        return when (val res = ports.api.createKeyAccount(name)) {
            is AuthApiResult.Err -> res
            is AuthApiResult.Ok -> {
                ports.keys.saveKey(KeyBackup(res.value.username, res.value.accountKey))
                res
            }
        }
    }

    /**
     * Key-only login: the account key is the single credential (no
     * email, provider or device key). Persists the session and the key
     * backup together; unknown and wrong keys share the server
     * `key-invalid` refusal.
     */
    fun loginWithKey(rawKey: String): AuthApiResult<Login> {
        if (rawKey.isBlank()) return AuthApiResult.Err(CLIENT_INVALID)
        return when (val res = ports.api.loginWithKey(rawKey.trim())) {
            is AuthApiResult.Err -> res
            is AuthApiResult.Ok -> {
                ports.store.save(res.value.session)
                ports.keys.saveKey(KeyBackup(res.value.username, rawKey.trim()))
                AuthApiResult.Ok(Login(res.value.session, created = false))
            }
        }
    }

    /** Stored key backup for the Profile backup screen, or null. */
    fun currentKey(): KeyBackup? = ports.keys.loadKey()

    /** Live session or null (expired sessions need [refreshSession]). */
    fun currentSession(): PortableAuth.Session? {
        val session = ports.store.load() ?: return null
        if (!PortableAuth.isAccessLive(session, ports.clock.nowEpochSeconds())) return null
        return session
    }

    /**
     * Returns a live session, rotating first when only the refresh
     * family survives. Null means re-login: no usable session remains.
     */
    fun refreshSession(): AuthApiResult<PortableAuth.Session?> {
        val session = ports.store.load() ?: return AuthApiResult.Ok(null)
        val now = ports.clock.nowEpochSeconds()
        if (PortableAuth.isAccessLive(session, now)) {
            return AuthApiResult.Ok(session)
        }
        if (!PortableAuth.isRefreshLive(session, now)) {
            ports.store.clear()
            return AuthApiResult.Ok(null)
        }
        return when (val res = ports.api.refresh(session.familyId, session.refreshToken)) {
            is AuthApiResult.Err -> res
            is AuthApiResult.Ok -> {
                ports.store.save(res.value)
                AuthApiResult.Ok(res.value)
            }
        }
    }

    /**
     * Logs out everywhere. Local storage always clears — even when the
     * revoke call fails offline — so the device never stays stuck; the
     * server family expires on its own. The API outcome still reports.
     * The stored key backup clears too: after logout only the
     * contributor-held copy can log back in.
     */
    fun logout(): AuthApiResult<Unit> {
        val session = ports.store.load()
        ports.store.clear()
        ports.keys.clearKey()
        if (session == null) return AuthApiResult.Ok(Unit)
        return ports.api.revokeAll(session.familyId, session.accessToken)
    }

    /**
     * Rehydrates state after start or process death from secure storage
     * alone: no I/O, only shape and expiry math.
     */
    fun rehydrate(): AuthState {
        val session = ports.store.load() ?: return AuthState.LoggedOut
        if (session.familyId.isEmpty() || session.accessToken.isEmpty() ||
            session.refreshToken.isEmpty() || session.accountId.isEmpty()
        ) {
            return AuthState.LoggedOut
        }
        val now = ports.clock.nowEpochSeconds()
        if (PortableAuth.isAccessLive(session, now)) return AuthState.Active(session)
        if (PortableAuth.isRefreshLive(session, now)) return AuthState.NeedsRefresh(session)
        return AuthState.LoggedOut
    }

    /**
     * Begins a provider login: mints a fresh single-use nonce plus an
     * anti-CSRF state and returns the attempt the native layer opens
     * the provider flow with.
     */
    fun beginProviderLogin(provider: String): AuthApiResult<PortableAuth.LoginAttempt> {
        if (!PortableAuth.isProvider(provider)) return AuthApiResult.Err(CLIENT_INVALID)
        val attempt = PortableAuth.LoginAttempt(
            provider = provider,
            nonce = ports.nonces.nextNonce(),
            state = ports.nonces.nextNonce(),
        )
        if (attempt.nonce.isEmpty() || attempt.state.isEmpty()) {
            return AuthApiResult.Err(CLIENT_INVALID)
        }
        return AuthApiResult.Ok(attempt)
    }

    /**
     * Completes a provider login against its attempt. Mismatched,
     * substituted or cross-provider callbacks refuse LOCALLY — the
     * proof is never sent — and spent nonces refuse replays.
     */
    fun completeProviderLogin(
        session: PortableAuth.Session,
        attempt: PortableAuth.LoginAttempt,
        callback: PortableAuth.ProviderCallback,
    ): AuthApiResult<Unit> {
        if (spentNonces.contains(callback.nonce)) {
            return AuthApiResult.Err(PortableAuth.Verdict.CODE_UNKNOWN)
        }
        val bound = PortableAuth.bind(attempt, callback)
            ?: return AuthApiResult.Err(CLIENT_INVALID)
        if (bound.idToken.length > MAX_PROOF_CHARS) {
            return AuthApiResult.Err(CLIENT_INVALID)
        }
        return when (val res = ports.api.linkProvider(session, bound.provider, bound.idToken, bound.nonce)) {
            is AuthApiResult.Err -> {
                // A consumed server nonce is spent client-side too, so a
                // blind retry cannot loop forever.
                if (res.verdict == PortableAuth.Verdict.CODE_UNKNOWN ||
                    res.verdict == "oidc-nonce-reused"
                ) {
                    spentNonces.add(callback.nonce)
                }
                res
            }
            is AuthApiResult.Ok -> {
                spentNonces.add(callback.nonce)
                res
            }
        }
    }

    /** Self-deletes the account and always clears local storage. */
    fun deleteAccount(): AuthApiResult<Unit> {
        val session = ports.store.load() ?: return AuthApiResult.Err(CLIENT_INVALID)
        ports.store.clear()
        ports.keys.clearKey()
        return ports.api.deleteAccount(session)
    }

    private val spentNonces: MutableSet<String> = mutableSetOf()

    private fun isEmailHint(address: String): Boolean {
        val trimmed = address.trim()
        if (trimmed.isEmpty() || trimmed.length > 254) return false
        val at = trimmed.indexOf('@')
        return at > 0 && at < trimmed.length - 1 && !trimmed.contains(' ')
    }

    private fun isUsernameHint(name: String): Boolean {
        if (name.length < 3 || name.length > 20) return false
        if (name[0] !in 'a'..'z') return false
        return name.all { it in 'a'..'z' || it in '0'..'9' }
    }
}

/** Narrow collaborators behind the portable auth flows. */
data class AuthPorts(
    val store: AuthSessionStore,
    val keys: AuthKeyStore,
    val clock: AuthWallClock,
    val nonces: PortableNonceSource,
    val api: AuthAccountApi,
)

/** Opaque secure-storage port (Keystore/Keychain adapters own it). */
interface AuthSessionStore {
    fun save(session: PortableAuth.Session)
    fun load(): PortableAuth.Session?
    fun clear()
}

/** Opaque account-key backup port; sealed exactly like sessions. */
interface AuthKeyStore {
    fun saveKey(backup: AuthFlow.KeyBackup)
    fun loadKey(): AuthFlow.KeyBackup?
    fun clearKey()
}

/** Wall-clock seconds for expiry math (monotonic tick stays in PortableClock). */
fun interface AuthWallClock {
    fun nowEpochSeconds(): Long
}

/** Server contract surface used by the flows; verdicts pass through verbatim. */
interface AuthAccountApi {
    fun requestCode(email: String): AuthApiResult<Unit>
    fun consumeCode(email: String, code: String): AuthApiResult<ConsumeOk>
    fun createKeyAccount(username: String): AuthApiResult<AuthFlow.KeyIssued>
    fun loginWithKey(accountKey: String): AuthApiResult<KeyLogin>
    fun refresh(familyId: String, refreshToken: String): AuthApiResult<PortableAuth.Session>
    fun revokeAll(familyId: String, accessToken: String): AuthApiResult<Unit>
    fun linkProvider(
        session: PortableAuth.Session,
        provider: String,
        idToken: String,
        nonce: String,
    ): AuthApiResult<Unit>
    fun deleteAccount(session: PortableAuth.Session): AuthApiResult<Unit>
}

/** Redeemed login: the session plus whether signup created the account. */
data class ConsumeOk(
    val session: PortableAuth.Session,
    val created: Boolean,
)

/** Key-only login: the session plus the username it belongs to. */
data class KeyLogin(
    val session: PortableAuth.Session,
    val username: String,
)

/** API outcome: success value or stable backend verdict, never thrown. */
sealed interface AuthApiResult<out T> {
    data class Ok<T>(val value: T) : AuthApiResult<T>
    data class Err(val verdict: String) : AuthApiResult<Nothing>
}
