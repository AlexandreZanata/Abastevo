package com.anpfuel.app.ui.auth

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.domain.portable.PortableAuth
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/** Visible auth step. Session tokens never render; the account key renders only on its one-display and backup screens. */
enum class AuthStep {
    CHECKING,
    EMAIL_ENTRY,
    CODE_SENT,
    USERNAME_ENTRY,
    KEY_ISSUED,
    KEY_ENTRY,
    BUSY,
    PROVIDER_PENDING,
    AUTHENTICATED,
    OFFLINE_ACCOUNT,
}

/** Locale-free auth failure for string mapping in the screen. */
sealed interface AuthUiError {
    data object InvalidInput : AuthUiError
    data object WrongCode : AuthUiError
    data object WrongKey : AuthUiError
    data object UsernameTaken : AuthUiError
    data object ExpiredCode : AuthUiError
    data object LockedCode : AuthUiError
    data object SessionExpired : AuthUiError
    data object Suspended : AuthUiError
    data object Deleted : AuthUiError
    data object Offline : AuthUiError
    data object ProviderDenied : AuthUiError
    data object LoginFirst : AuthUiError
    data object SecureStorage : AuthUiError
    data object Unknown : AuthUiError
}

data class AuthUiState(
    val step: AuthStep = AuthStep.CHECKING,
    val email: String = "",
    val code: String = "",
    val username: String = "",
    val accountKey: String = "",
    val issuedUsername: String = "",
    val issuedKey: String = "",
    val keyBackup: AuthFlow.KeyBackup? = null,
    val pendingProvider: String = "",
    val error: AuthUiError? = null,
)

sealed interface AuthNavigation {
    data object NavigateBack : AuthNavigation
}

/**
 * FREE-account login ViewModel (P13-T05C).
 *
 * Thin coroutine shell over the portable [AuthFlow]: all ceremony rules
 * live there and are JVM-tested; here only step transitions, locale-free
 * error mapping and navigation events remain. Provider buttons create a
 * bound attempt (first ceremony half); completion arrives via the
 * `anpfuel://auth/callback` deep link handled by [onProviderCallback].
 * The native provider SDK that mints real id tokens is device-gated
 * (release horizon); until then completion runs in tests and reviews.
 */
@HiltViewModel
class AuthViewModel @Inject constructor(
    private val authFlow: AuthFlow,
) : ViewModel() {

    private val _uiState = MutableStateFlow(AuthUiState())
    val uiState: StateFlow<AuthUiState> = _uiState.asStateFlow()

    private val _navigation = MutableSharedFlow<AuthNavigation>(extraBufferCapacity = 1)
    val navigation: SharedFlow<AuthNavigation> = _navigation.asSharedFlow()

    private var refreshGeneration: Int = 0

    private var pendingAttempt: PortableAuth.LoginAttempt? = null

    /** Test seam for the IO context (unit tests inject the test dispatcher). */
    internal var ioDispatcher: kotlinx.coroutines.CoroutineDispatcher = Dispatchers.IO

    /**
     * Blocking account HTTP runs on IO: the portable flows are
     * synchronous by contract (threading stays with the caller) and
     * Main-thread network throws NetworkOnMainThreadException, which
     * the UI would misread as "no connection".
     */
    private suspend fun <T> onIo(block: suspend () -> T): T =
        withContext(ioDispatcher) { block() }

    init {
        refreshAccount()
    }

    /** Reconcile shared secure storage when a retained destination resumes. */
    fun refreshAccount() {
        if (_uiState.value.step == AuthStep.BUSY || pendingAttempt != null) return
        val generation = ++refreshGeneration
        viewModelScope.launch {
            val state = onIo { authFlow.rehydrate() }
            if (generation != refreshGeneration) return@launch
            when (state) {
                is AuthFlow.AuthState.Active -> {
                    _uiState.update {
                        if (it.step == AuthStep.KEY_ISSUED && it.issuedKey.isNotEmpty()) {
                            it.copy(keyBackup = authFlow.currentKey())
                        } else AuthUiState(step = AuthStep.AUTHENTICATED, keyBackup = authFlow.currentKey())
                    }
                }
                is AuthFlow.AuthState.PendingKeyAccount -> {
                    _uiState.update {
                        AuthUiState(step = AuthStep.KEY_ISSUED, issuedUsername = state.backup.username,
                            issuedKey = state.backup.accountKey)
                    }
                }
                is AuthFlow.AuthState.NeedsRefresh -> {
                    val res = onIo { authFlow.refreshSession() }
                    if (generation != refreshGeneration) return@launch
                    when (res) {
                        is AuthApiResult.Ok -> _uiState.update {
                            AuthUiState(step = if (res.value == null) AuthStep.USERNAME_ENTRY else AuthStep.AUTHENTICATED,
                                keyBackup = if (res.value == null) null else authFlow.currentKey())
                        }
                        is AuthApiResult.Err -> _uiState.update {
                            AuthUiState(
                                step = if (authFlow.rehydrate() is AuthFlow.AuthState.NeedsRefresh) {
                                    AuthStep.OFFLINE_ACCOUNT
                                } else AuthStep.USERNAME_ENTRY,
                                keyBackup = authFlow.currentKey(), error = mapError(res.verdict),
                            )
                        }
                    }
                }
                is AuthFlow.AuthState.LoggedOut -> {
                    AccountKeyUnlockSession.grant.reset()
                    if (_uiState.value.step in listOf(AuthStep.CHECKING, AuthStep.AUTHENTICATED, AuthStep.OFFLINE_ACCOUNT)) {
                        _uiState.update { AuthUiState(step = AuthStep.USERNAME_ENTRY) }
                    }
                }
            }
        }
    }

    fun onUsernameChange(value: String) {
        _uiState.update { it.copy(username = value.lowercase().filter { c -> c in 'a'..'z' || c in '0'..'9' }.take(20), error = null) }
    }

    fun onKeyChange(value: String) {
        _uiState.update { it.copy(accountKey = value.take(96), error = null) }
    }

    fun onGoToKeyEntry() {
        if (_uiState.value.step == AuthStep.BUSY) return
        _uiState.update { it.copy(step = AuthStep.KEY_ENTRY, error = null) }
    }

    fun onBackToUsername() {
        if (_uiState.value.step == AuthStep.BUSY) return
        _uiState.update { it.copy(step = AuthStep.USERNAME_ENTRY, accountKey = "", issuedKey = "", keyBackup = null, error = null) }
    }

    /** Create, sign in, then keep the issued-key backup step visible. */
    fun onCreateAccount() {
        AccountKeyUnlockSession.grant.reset()
        if (_uiState.value.step != AuthStep.USERNAME_ENTRY) return
        refreshGeneration++
        val username = _uiState.value.username
        _uiState.update { it.copy(step = AuthStep.BUSY, error = null) }
        viewModelScope.launch {
            when (val res = onIo { authFlow.createKeyAccount(username) }) {
                is AuthApiResult.Ok -> {
                    val login = onIo { authFlow.loginWithKey(res.value.accountKey) }
                    _uiState.update {
                        it.copy(
                            step = AuthStep.KEY_ISSUED,
                            error = (login as? AuthApiResult.Err)?.let { failure -> mapError(failure.verdict) },
                            issuedUsername = res.value.username,
                            issuedKey = res.value.accountKey,
                            keyBackup = authFlow.currentKey(),
                        )
                    }
                }
                is AuthApiResult.Err -> {
                    _uiState.update {
                        it.copy(step = AuthStep.USERNAME_ENTRY, error = mapError(res.verdict))
                    }
                }
            }
        }
    }

    /** Key-only login, including the enter-after-signup step. */
    fun onLoginWithKey() {
        val snapshot = _uiState.value
        if (snapshot.step !in listOf(AuthStep.KEY_ENTRY, AuthStep.KEY_ISSUED)) return
        AccountKeyUnlockSession.grant.reset()
        refreshGeneration++
        val key = snapshot.accountKey.ifEmpty { snapshot.issuedKey }
        _uiState.update { it.copy(step = AuthStep.BUSY, error = null) }
        viewModelScope.launch {
            when (val res = onIo {
                val session = if (snapshot.step == AuthStep.KEY_ISSUED) authFlow.currentSession() else null
                if (session == null) authFlow.loginWithKey(key) else AuthApiResult.Ok(AuthFlow.Login(session, true))
            }) {
                is AuthApiResult.Ok -> {
                    _uiState.update {
                        it.copy(
                            step = AuthStep.AUTHENTICATED,
                            accountKey = "",
                            issuedKey = "",
                            keyBackup = authFlow.currentKey(),
                            error = null,
                        )
                    }
                    _navigation.emit(AuthNavigation.NavigateBack)
                }
                is AuthApiResult.Err -> {
                    _uiState.update {
                        it.copy(step = snapshot.step, error = mapError(res.verdict))
                    }
                }
            }
        }
    }

    /** Refreshes the locally stored key backup for the Profile screen. */
    fun loadBackup() {
        _uiState.update { it.copy(keyBackup = authFlow.currentKey()) }
    }

    fun onEmailChange(value: String) {
        _uiState.update { it.copy(email = value, error = null) }
    }

    fun onCodeChange(value: String) {
        _uiState.update { it.copy(code = value.filter { c -> c.isDigit() }.take(6), error = null) }
    }

    fun onRequestCode() {
        val email = _uiState.value.email
        _uiState.update { it.copy(step = AuthStep.BUSY, error = null) }
        viewModelScope.launch {
            when (val res = onIo { authFlow.requestEmailCode(email) }) {
                is AuthApiResult.Ok -> {
                    _uiState.update { it.copy(step = AuthStep.CODE_SENT) }
                }
                is AuthApiResult.Err -> {
                    _uiState.update {
                        it.copy(step = AuthStep.EMAIL_ENTRY, error = mapError(res.verdict))
                    }
                }
            }
        }
    }

    fun onConsumeCode() {
        val snapshot = _uiState.value
        _uiState.update { it.copy(step = AuthStep.BUSY, error = null) }
        viewModelScope.launch {
            when (val res = onIo { authFlow.consumeEmailCode(snapshot.email, snapshot.code) }) {
                is AuthApiResult.Ok -> {
                    _uiState.update {
                        it.copy(step = AuthStep.AUTHENTICATED, code = "", error = null)
                    }
                    _navigation.emit(AuthNavigation.NavigateBack)
                }
                is AuthApiResult.Err -> {
                    _uiState.update {
                        it.copy(step = AuthStep.CODE_SENT, error = mapError(res.verdict))
                    }
                }
            }
        }
    }

    fun onBackToEmail() {
        pendingAttempt = null
        _uiState.update {
            it.copy(step = AuthStep.EMAIL_ENTRY, code = "", error = null)
        }
    }

    fun onProviderClick(provider: String) {
        _uiState.update { it.copy(error = null) }
        viewModelScope.launch {
            when (val res = authFlow.beginProviderLogin(provider)) {
                is AuthApiResult.Ok -> {
                    pendingAttempt = res.value
                    _uiState.update {
                        it.copy(step = AuthStep.PROVIDER_PENDING, pendingProvider = provider)
                    }
                }
                is AuthApiResult.Err -> {
                    _uiState.update { it.copy(error = mapError(res.verdict)) }
                }
            }
        }
    }

    fun onCancelProviderLink() {
        pendingAttempt = null
        viewModelScope.launch {
            val step = if (authFlow.currentSession() == null) {
                AuthStep.USERNAME_ENTRY
            } else {
                AuthStep.AUTHENTICATED
            }
            _uiState.update { it.copy(step = step, pendingProvider = "", error = null) }
        }
    }

    fun onProviderCallback(provider: String, idToken: String, nonce: String, state: String) {
        val attempt = pendingAttempt
        if (attempt == null) {
            _uiState.update { it.copy(error = AuthUiError.InvalidInput) }
            return
        }
        val session = authFlow.currentSession()
        if (session == null) {
            _uiState.update { it.copy(error = AuthUiError.LoginFirst) }
            return
        }
        _uiState.update { it.copy(error = null) }
        viewModelScope.launch {
            val callback = PortableAuth.ProviderCallback(provider, idToken, nonce, state)
            when (val res = onIo { authFlow.completeProviderLogin(session, attempt, callback) }) {
                is AuthApiResult.Ok -> {
                    pendingAttempt = null
                    _uiState.update {
                        it.copy(step = AuthStep.AUTHENTICATED, pendingProvider = "")
                    }
                }
                is AuthApiResult.Err -> {
                    _uiState.update { it.copy(error = mapError(res.verdict)) }
                }
            }
        }
    }

    fun onLogout() {
        AccountKeyUnlockSession.grant.reset()
        refreshGeneration++
        _uiState.value = AuthUiState(step = AuthStep.BUSY)
        viewModelScope.launch {
            onIo { authFlow.logout() }
            pendingAttempt = null
            _uiState.update {
                AuthUiState(step = AuthStep.USERNAME_ENTRY)
            }
        }
    }

    /**
     * P22-T01 — self-deletes the account (server `POST
     * /v1/accounts/deletion` plus local storage wipe inside
     * [AuthFlow.deleteAccount]). Success returns to username entry with the
     * Deleted notice; transport failure keeps the account and backup.
     * Definitive auth denials clear the displayed authority.
     */
    fun onDeleteAccount() {
        AccountKeyUnlockSession.grant.reset()
        refreshGeneration++
        _uiState.value = AuthUiState(step = AuthStep.BUSY)
        viewModelScope.launch {
            when (val res = onIo { authFlow.deleteAccount() }) {
                is AuthApiResult.Ok -> {
                    pendingAttempt = null
                    _uiState.update {
                        AuthUiState(step = AuthStep.USERNAME_ENTRY, error = AuthUiError.Deleted)
                    }
                }
                is AuthApiResult.Err -> {
                    val retained = onIo { authFlow.rehydrate() }
                    _uiState.value = AuthUiState(
                        step = when (retained) {
                            is AuthFlow.AuthState.Active -> AuthStep.AUTHENTICATED
                            is AuthFlow.AuthState.NeedsRefresh -> AuthStep.OFFLINE_ACCOUNT
                            else -> AuthStep.USERNAME_ENTRY
                        },
                        keyBackup = if (retained is AuthFlow.AuthState.Active || retained is AuthFlow.AuthState.NeedsRefresh) {
                            authFlow.currentKey()
                        } else null,
                        error = mapError(res.verdict),
                    )
                }
            }
        }
    }

    fun onDismissError() {
        _uiState.update { it.copy(error = null) }
    }

    companion object {
        /** Backend verdicts (plus the local-hint code) to UI errors. */
        fun mapError(verdict: String): AuthUiError = when (verdict) {
            AuthFlow.CLIENT_INVALID -> AuthUiError.InvalidInput
            AuthFlow.SECURE_STORAGE_UNAVAILABLE -> AuthUiError.SecureStorage
            PortableAuth.Verdict.CODE_UNKNOWN,
            PortableAuth.Verdict.CODE_CONSUMED,
            -> AuthUiError.WrongCode
            "key-invalid" -> AuthUiError.WrongKey
            "username-taken" -> AuthUiError.UsernameTaken
            "username-invalid" -> AuthUiError.InvalidInput
            PortableAuth.Verdict.CODE_EXPIRED -> AuthUiError.ExpiredCode
            PortableAuth.Verdict.CODE_LOCKED -> AuthUiError.LockedCode
            PortableAuth.Verdict.SESSION_REUSE,
            PortableAuth.Verdict.SESSION_REVOKED,
            PortableAuth.Verdict.SESSION_EXPIRED,
            -> AuthUiError.SessionExpired
            PortableAuth.Verdict.ACCOUNT_SUSPENDED -> AuthUiError.Suspended
            PortableAuth.Verdict.ACCOUNT_DELETED -> AuthUiError.Deleted
            PortableAuth.UNAVAILABLE -> AuthUiError.Offline
            else -> {
                // Provider/link ceremony verdicts (oidc-*, link-*) share
                // one denial: the proof failed, never which check fired.
                if (verdict.startsWith("oidc-") || verdict.startsWith("link-")) {
                    AuthUiError.ProviderDenied
                } else {
                    AuthUiError.Unknown
                }
            }
        }
    }
}
