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
import kotlinx.coroutines.launch

/** Visible auth step. Tokens never appear in state, logs or UI. */
enum class AuthStep {
    CHECKING,
    EMAIL_ENTRY,
    CODE_SENT,
    BUSY,
    PROVIDER_PENDING,
    AUTHENTICATED,
}

/** Locale-free auth failure for string mapping in the screen. */
sealed interface AuthUiError {
    data object InvalidInput : AuthUiError
    data object WrongCode : AuthUiError
    data object ExpiredCode : AuthUiError
    data object LockedCode : AuthUiError
    data object SessionExpired : AuthUiError
    data object Suspended : AuthUiError
    data object Deleted : AuthUiError
    data object Offline : AuthUiError
    data object ProviderDenied : AuthUiError
    data object LoginFirst : AuthUiError
    data object Unknown : AuthUiError
}

data class AuthUiState(
    val step: AuthStep = AuthStep.CHECKING,
    val email: String = "",
    val code: String = "",
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

    private var pendingAttempt: PortableAuth.LoginAttempt? = null

    init {
        viewModelScope.launch {
            when (val state = authFlow.rehydrate()) {
                is AuthFlow.AuthState.Active -> {
                    _uiState.update { it.copy(step = AuthStep.AUTHENTICATED) }
                }
                is AuthFlow.AuthState.NeedsRefresh -> {
                    when (val res = authFlow.refreshSession()) {
                        is AuthApiResult.Ok -> {
                            if (res.value == null) {
                                _uiState.update { it.copy(step = AuthStep.EMAIL_ENTRY) }
                            } else {
                                _uiState.update { it.copy(step = AuthStep.AUTHENTICATED) }
                            }
                        }
                        is AuthApiResult.Err -> {
                            _uiState.update {
                                it.copy(step = AuthStep.EMAIL_ENTRY, error = mapError(res.verdict))
                            }
                        }
                    }
                }
                is AuthFlow.AuthState.LoggedOut -> {
                    _uiState.update { it.copy(step = AuthStep.EMAIL_ENTRY) }
                }
            }
        }
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
            when (val res = authFlow.requestEmailCode(email)) {
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
            when (val res = authFlow.consumeEmailCode(snapshot.email, snapshot.code)) {
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
                AuthStep.EMAIL_ENTRY
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
            when (val res = authFlow.completeProviderLogin(session, attempt, callback)) {
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
        viewModelScope.launch {
            authFlow.logout()
            pendingAttempt = null
            _uiState.update {
                AuthUiState(step = AuthStep.EMAIL_ENTRY)
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
            PortableAuth.Verdict.CODE_UNKNOWN,
            PortableAuth.Verdict.CODE_CONSUMED,
            -> AuthUiError.WrongCode
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
