package com.anpfuel.application.portable

import com.anpfuel.domain.portable.PortableAuth

/**
 * Orchestrates free account access and secure local custody. The account API
 * enforces the email proofs, provider identity and session restrictions.
 */
class AuthFlow(private val ports: AuthPorts) {

    /** Local-hint refusal code; never sent, never a backend verdict. */
    companion object {
        const val CLIENT_INVALID: String = "client-invalid"
        const val SECURE_STORAGE_UNAVAILABLE: String = "secure-storage-unavailable"

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

    /** The encrypted backup label retains the historical username field. */
    data class KeyBackup(
        val username: String,
        val accountKey: String,
    )

    /** Rehydrated state after start or process death. */
    sealed interface AuthState {
        data object LoggedOut : AuthState
        data class PendingKeyAccount(val backup: KeyBackup) : AuthState
        data class Active(val session: PortableAuth.Session) : AuthState
        data class NeedsRefresh(val session: PortableAuth.Session) : AuthState
    }

    /** Requests an access code. Unknown addresses answer identically. */
    fun requestEmailCode(email: String): AuthApiResult<Unit> {
        if (!isEmailHint(email)) return AuthApiResult.Err(CLIENT_INVALID)
        return ports.api.requestCode(email.trim())
    }

    fun requestCode(email: String, purpose: PortableAuth.CodePurpose, locale: String): AuthApiResult<Unit> {
        if (!isEmailHint(email)) return AuthApiResult.Err(CLIENT_INVALID)
        return ports.api.requestCode(email.trim(), purpose, locale)
    }

    fun signUp(name: String, email: String, code: String): AuthApiResult<NewKey> =
        issueKey(email, code) { address ->
            if (name.isBlank()) AuthApiResult.Err(PortableAuth.Verdict.NAME_INVALID)
            else ports.api.signUp(name, address, code)
        }

    fun recover(email: String, code: String): AuthApiResult<NewKey> =
        issueKey(email, code) { address -> ports.api.recover(address, code) }

    /** Retain the only display of a new key even if durable storage fails. */
    data class NewKey(val backup: KeyBackup, val sessionSaved: Boolean)

    private fun issueKey(
        email: String,
        code: String,
        request: (String) -> AuthApiResult<IssuedAccountKey>,
    ): AuthApiResult<NewKey> = ports.lock.withLock {
        if (!isEmailHint(email) || !PortableAuth.isCodeShape(code)) {
            return@withLock AuthApiResult.Err(CLIENT_INVALID)
        }
        when (val result = request(email.trim())) {
            is AuthApiResult.Err -> result
            is AuthApiResult.Ok -> {
                val issued = result.value
                val backup = KeyBackup(issued.publicName, issued.accountKey)
                val saved = saveKey(backup)
                if (!saved) {
                    // Recovery has invalidated the preceding key. Keeping it
                    // would misrepresent which backup the device can use.
                    try { ports.keys.clearKey() } catch (_: Exception) { }
                }
                val sessionSaved = saved && persistSession(issued.session)
                if (!sessionSaved) {
                    try { ports.store.clear() } catch (_: Exception) { }
                }
                AuthApiResult.Ok(NewKey(backup, sessionSaved))
            }
        }
    }

    fun loginWithGoogle(
        idToken: String,
        rawNonce: String,
        expectedAccountId: String? = null,
    ): AuthApiResult<GoogleLogin> = ports.lock.withLock {
        if (!validGoogleProof(idToken, rawNonce)) return@withLock AuthApiResult.Err(CLIENT_INVALID)
        val previous = ports.store.load()
        if (expectedAccountId != null && previous?.accountId != expectedAccountId) {
            return@withLock AuthApiResult.Err(CLIENT_INVALID)
        }
        when (val result = ports.api.loginWithGoogle(idToken, rawNonce)) {
            is AuthApiResult.Err -> result
            is AuthApiResult.Ok -> {
                val session = result.value.session
                if (expectedAccountId != null && expectedAccountId != session.accountId) {
                    return@withLock AuthApiResult.Err(CLIENT_INVALID)
                }
                if (previous?.accountId != session.accountId && ports.keys.loadKey() != null) {
                    try {
                        ports.keys.clearKey()
                    } catch (_: Exception) {
                        return@withLock AuthApiResult.Err(SECURE_STORAGE_UNAVAILABLE)
                    }
                }
                if (persistSession(session)) result else AuthApiResult.Err(SECURE_STORAGE_UNAVAILABLE)
            }
        }
    }

    fun profile(): AuthApiResult<PortableAuth.Profile> = withSession(ports.api::profile)

    fun updateName(name: String): AuthApiResult<PortableAuth.Profile> {
        if (name.isBlank()) return AuthApiResult.Err(PortableAuth.Verdict.NAME_INVALID)
        return withSession { ports.api.updateName(it, name) }
    }

    fun setRecoveryEmail(email: String, code: String): AuthApiResult<PortableAuth.Profile> {
        if (!isEmailHint(email) || !PortableAuth.isCodeShape(code)) return AuthApiResult.Err(CLIENT_INVALID)
        return withSession { ports.api.setRecoveryEmail(it, email.trim(), code) }
    }

    fun linkGoogle(idToken: String, rawNonce: String): AuthApiResult<Unit> {
        if (!validGoogleProof(idToken, rawNonce)) return AuthApiResult.Err(CLIENT_INVALID)
        return withSession { ports.api.linkGoogle(it, idToken, rawNonce) }
    }

    private fun validGoogleProof(idToken: String, rawNonce: String): Boolean =
        idToken.isNotBlank() && idToken.length <= 4096 &&
            rawNonce.length in 16..128 && rawNonce.all { it in '!'..'~' }

    private fun <T> withSession(request: (PortableAuth.Session) -> AuthApiResult<T>): AuthApiResult<T> =
        ports.lock.withLock {
            when (val session = refreshSession()) {
                is AuthApiResult.Err -> session
                is AuthApiResult.Ok -> session.value?.let(request)
                    ?: AuthApiResult.Err(PortableAuth.Verdict.SESSION_EXPIRED)
            }
        }

    /**
     * Redeems one code for a session and persists it. Wrong codes share
     * the server `code-unknown` refusal; suspended accounts refuse
     * without burning the code (server-side, mirrored here by exact
     * verdict passthrough).
     */
    fun consumeEmailCode(email: String, code: String): AuthApiResult<Login> = ports.lock.withLock {
        // Export and explicitly log out before switching away from a key account.
        // A legacy email ceremony must not overwrite its only recovery key.
        if (ports.keys.loadKey() != null) return@withLock AuthApiResult.Err(CLIENT_INVALID)
        if (!isEmailHint(email) || !PortableAuth.isCodeShape(code)) {
            return@withLock AuthApiResult.Err(CLIENT_INVALID)
        }
        return@withLock when (val res = ports.api.consumeCode(email.trim(), code)) {
            is AuthApiResult.Err -> res
            is AuthApiResult.Ok -> {
                if (persistSession(res.value.session)) AuthApiResult.Ok(Login(res.value.session, res.value.created))
                else AuthApiResult.Err(SECURE_STORAGE_UNAVAILABLE)
            }
        }
    }

    /**
     * Creates an anonymous key account for username and persists the
     * one-display key for backup. Creation alone logs nothing in: the
     * contributor enters with the key via [loginWithKey], so the backup
     * moment cannot be skipped.
     */
    fun createKeyAccount(username: String): AuthApiResult<KeyIssued> = ports.lock.withLock {
        val name = username.trim().lowercase()
        if (!isUsernameHint(name)) return@withLock AuthApiResult.Err(CLIENT_INVALID)
        return@withLock when (val res = ports.api.createKeyAccount(name)) {
            is AuthApiResult.Err -> res
            is AuthApiResult.Ok -> {
                // Always return the only issued key, including storage failure,
                // so the UI can retain/reveal it for recovery rather than lose it.
                try {
                    ports.keys.saveKey(KeyBackup(res.value.username, res.value.accountKey))
                } catch (_: Exception) {
                    // loginWithKey verifies durable custody and reports failure.
                }
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
    fun loginWithKey(rawKey: String, expectedAccountId: String? = null): AuthApiResult<Login> = ports.lock.withLock {
        if (rawKey.isBlank() || (expectedAccountId != null && ports.store.load()?.accountId != expectedAccountId)) {
            return@withLock AuthApiResult.Err(CLIENT_INVALID)
        }
        return@withLock when (val res = ports.api.loginWithKey(rawKey.trim())) {
            is AuthApiResult.Err -> res
            is AuthApiResult.Ok -> {
                if (expectedAccountId != null && res.value.session.accountId != expectedAccountId) {
                    return@withLock AuthApiResult.Err(CLIENT_INVALID)
                }
                val backup = KeyBackup(res.value.username, rawKey.trim())
                val previousBackup = ports.keys.loadKey()
                val previousSession = ports.store.load()
                if (!saveKey(backup) || !persistSession(res.value.session)) {
                    // Preserve a previously issued recovery key on write failure.
                    try {
                        if (previousBackup != null) ports.keys.saveKey(previousBackup)
                        if (previousSession != null) ports.store.save(previousSession)
                    } catch (_: Exception) {
                        // The storage failure is reported; never grant navigation.
                    }
                    AuthApiResult.Err(SECURE_STORAGE_UNAVAILABLE)
                } else AuthApiResult.Ok(Login(res.value.session, created = false))
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
    fun refreshSession(): AuthApiResult<PortableAuth.Session?> = ports.lock.withLock {
        val session = ports.store.load() ?: return@withLock AuthApiResult.Ok(null)
        val now = ports.clock.nowEpochSeconds()
        if (PortableAuth.isAccessLive(session, now) && session.accessExpiresAt - now > 60L) {
            return@withLock AuthApiResult.Ok(session)
        }
        val backup = ports.keys.loadKey()
        if (!PortableAuth.isRefreshLive(session, now) && backup == null) {
            return@withLock if (clearLocalAuth()) AuthApiResult.Ok(null)
            else AuthApiResult.Err(SECURE_STORAGE_UNAVAILABLE)
        }
        return@withLock when (val res = ports.api.refresh(session.familyId, session.refreshToken)) {
            is AuthApiResult.Err -> {
                if (res.verdict == PortableAuth.Verdict.SESSION_EXPIRED && backup != null) {
                    // Ask the old family first: a revoked/reused family must
                    // never silently regain authority through the saved key.
                    when (val login = ports.api.loginWithKey(backup.accountKey)) {
                        is AuthApiResult.Err -> {
                            if (isAuthDenial(login.verdict)) clearAndDeny(login.verdict) else login
                        }
                        is AuthApiResult.Ok -> {
                            if (login.value.session.accountId != session.accountId) {
                                clearAndDeny(CLIENT_INVALID)
                            } else {
                                if (persistSession(login.value.session)) AuthApiResult.Ok(login.value.session)
                                else AuthApiResult.Err(SECURE_STORAGE_UNAVAILABLE)
                            }
                        }
                    }
                } else {
                    if (isAuthDenial(res.verdict)) clearAndDeny(res.verdict) else res
                }
            }
            is AuthApiResult.Ok -> {
                if (res.value.accountId != session.accountId) {
                    clearAndDeny(CLIENT_INVALID)
                } else {
                    if (persistSession(res.value)) AuthApiResult.Ok(res.value)
                    else AuthApiResult.Err(SECURE_STORAGE_UNAVAILABLE)
                }
            }
        }
    }

    private fun saveKey(backup: KeyBackup): Boolean = try {
        ports.keys.saveKey(backup)
        ports.keys.loadKey() == backup
    } catch (_: Exception) {
        false
    }

    private fun persistSession(session: PortableAuth.Session): Boolean {
        return try {
            ports.store.save(session)
            ports.store.load() == session
        } catch (_: Exception) {
            false
        }
    }

    private fun isAuthDenial(verdict: String): Boolean = verdict in listOf(
        PortableAuth.Verdict.SESSION_REVOKED, PortableAuth.Verdict.SESSION_REUSE,
        PortableAuth.Verdict.SESSION_INVALID, PortableAuth.Verdict.SESSION_EXPIRED,
        PortableAuth.Verdict.ACCOUNT_SUSPENDED,
        PortableAuth.Verdict.ACCOUNT_DELETED, PortableAuth.Verdict.CODE_UNKNOWN,
        "key-invalid", "account-not-found", CLIENT_INVALID,
    )

    private fun clearLocalAuth(): Boolean {
        var success = true
        try { ports.store.clear() } catch (_: Exception) { success = false }
        try { ports.keys.clearKey() } catch (_: Exception) { success = false }
        return success
    }

    private fun clearAndDeny(verdict: String): AuthApiResult.Err =
        AuthApiResult.Err(if (clearLocalAuth()) verdict else SECURE_STORAGE_UNAVAILABLE)

    /**
     * Attempts both durable local clears and remote revocation. A local
     * failure reports secure-storage-unavailable, never false logout success;
     * an offline remote failure still leaves successfully cleared local auth.
     * The stored key backup clears too: after logout only the
     * contributor-held copy can log back in.
     */
    fun logout(): AuthApiResult<Unit> = ports.lock.withLock {
        val session = ports.store.load()
        val cleared = clearLocalAuth()
        val result = if (session == null) AuthApiResult.Ok(Unit)
        else ports.api.revokeAll(session.familyId, session.accessToken)
        return@withLock if (cleared) result else AuthApiResult.Err(SECURE_STORAGE_UNAVAILABLE)
    }

    /**
     * Rehydrates state after start or process death from secure storage
     * alone: no I/O, only shape and expiry math.
     */
    fun rehydrate(): AuthState {
        val session = ports.store.load() ?: return ports.keys.loadKey()?.let {
            AuthState.PendingKeyAccount(it)
        } ?: AuthState.LoggedOut
        if (session.familyId.isEmpty() || session.accessToken.isEmpty() ||
            session.refreshToken.isEmpty() || session.accountId.isEmpty()
        ) {
            return AuthState.LoggedOut
        }
        val now = ports.clock.nowEpochSeconds()
        if (PortableAuth.isAccessLive(session, now)) return AuthState.Active(session)
        if (PortableAuth.isRefreshLive(session, now) || ports.keys.loadKey() != null) {
            return AuthState.NeedsRefresh(session)
        }
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

    /** Preserve recovery credentials until deletion succeeds or authority is denied. */
    fun deleteAccount(): AuthApiResult<Unit> = ports.lock.withLock {
        val session = when (val renewed = refreshSession()) {
            is AuthApiResult.Err -> return@withLock renewed
            is AuthApiResult.Ok -> renewed.value ?: return@withLock AuthApiResult.Err(CLIENT_INVALID)
        }
        val result = ports.api.deleteAccount(session)
        if (result is AuthApiResult.Ok || (result is AuthApiResult.Err && isAuthDenial(result.verdict))) {
            if (!clearLocalAuth()) return@withLock AuthApiResult.Err(SECURE_STORAGE_UNAVAILABLE)
        }
        return@withLock result
    }

    private val spentNonces: MutableSet<String> = mutableSetOf()

    private fun isEmailHint(address: String): Boolean {
        if (address.any { it.isISOControl() }) return false
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
    val lock: AuthOperationLock = DirectAuthOperationLock,
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
    fun requestCode(email: String, purpose: PortableAuth.CodePurpose, locale: String): AuthApiResult<Unit> =
        AuthApiResult.Err(PortableAuth.UNAVAILABLE)
    fun signUp(name: String, email: String, code: String): AuthApiResult<IssuedAccountKey> =
        AuthApiResult.Err(PortableAuth.UNAVAILABLE)
    fun recover(email: String, code: String): AuthApiResult<IssuedAccountKey> =
        AuthApiResult.Err(PortableAuth.UNAVAILABLE)
    fun loginWithGoogle(idToken: String, rawNonce: String): AuthApiResult<GoogleLogin> =
        AuthApiResult.Err(PortableAuth.UNAVAILABLE)
    fun profile(session: PortableAuth.Session): AuthApiResult<PortableAuth.Profile> =
        AuthApiResult.Err(PortableAuth.UNAVAILABLE)
    fun updateName(session: PortableAuth.Session, name: String): AuthApiResult<PortableAuth.Profile> =
        AuthApiResult.Err(PortableAuth.UNAVAILABLE)
    fun setRecoveryEmail(session: PortableAuth.Session, email: String, code: String): AuthApiResult<PortableAuth.Profile> =
        AuthApiResult.Err(PortableAuth.UNAVAILABLE)
    fun linkGoogle(session: PortableAuth.Session, idToken: String, rawNonce: String): AuthApiResult<Unit> =
        AuthApiResult.Err(PortableAuth.UNAVAILABLE)
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

data class IssuedAccountKey(
    val session: PortableAuth.Session,
    val publicName: String,
    val accountKey: String,
)

data class GoogleLogin(
    val session: PortableAuth.Session,
    val created: Boolean,
    val profileComplete: Boolean,
)

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

/** Serializes load/network/save against logout; Android supplies a reentrant lock. */
interface AuthOperationLock {
    fun <T> withLock(operation: () -> T): T
}

/** Deterministic single-thread default for portable consumers and tests. */
object DirectAuthOperationLock : AuthOperationLock {
    override fun <T> withLock(operation: () -> T): T = operation()
}
