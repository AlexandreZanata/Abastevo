package com.anpfuel.app.ui.auth

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.GoogleSignInGateway
import com.anpfuel.application.portable.GoogleSignInResult
import com.anpfuel.data.repository.OwnerLocalErasure
import com.anpfuel.domain.account.GoogleNonce
import com.anpfuel.domain.account.PublicName
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.portable.PortableAuth.CodePurpose
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

enum class AuthStep {
    CHECKING,
    ENTRY,
    SIGNUP_FORM,
    RECOVERY_FORM,
    CODE_SENT,
    KEY_ISSUED,
    KEY_ENTRY,
    BUSY,
    AUTHENTICATED,
    OFFLINE_ACCOUNT,
    CONFIRM_NAME,
    EDIT_NAME,
    ATTACH_EMAIL,
    REAUTHENTICATION,
    LINK_GOOGLE,
}

sealed interface AuthUiError {
    data object InvalidInput : AuthUiError
    data object WrongCode : AuthUiError
    data object WrongKey : AuthUiError
    data object ExpiredCode : AuthUiError
    data object LockedCode : AuthUiError
    data object SessionExpired : AuthUiError
    data object Suspended : AuthUiError
    data object Deleted : AuthUiError
    data object Offline : AuthUiError
    data object ProviderDenied : AuthUiError
    data object SecureStorage : AuthUiError
    data object Unknown : AuthUiError
    data object InvalidName : AuthUiError
    data object AddressLinked : AuthUiError
    data object RecoveryNoAccount : AuthUiError
    data object ProviderEmailInUse : AuthUiError
    data object ReauthenticationRequired : AuthUiError
    data object EmailLoginRetired : AuthUiError
    data object GoogleUnavailable : AuthUiError
}

data class AuthUiState(
    val step: AuthStep = AuthStep.CHECKING,
    val busyOrigin: AuthStep = AuthStep.CHECKING,
    val email: String = "",
    val code: String = "",
    val name: String = "",
    val accountKey: String = "",
    val issuedUsername: String = "",
    val issuedKey: String = "",
    val issuedKeyCanBeRecovered: Boolean = false,
    val keyBackup: AuthFlow.KeyBackup? = null,
    val profile: PortableAuth.Profile? = null,
    val purpose: CodePurpose = CodePurpose.SIGNUP,
    val resendSeconds: Int = 0,
    val showProtectionPrompt: Boolean = false,
    val canReauthenticateWithGoogle: Boolean = false,
    val error: AuthUiError? = null,
)

sealed interface AuthNavigation {
    data object NavigateBack : AuthNavigation
}

/** Drives the access screens while [AuthFlow] owns proof, session and key custody. */
@HiltViewModel
class AuthViewModel @Inject constructor(
    private val authFlow: AuthFlow,
    private val ownerErasure: OwnerLocalErasure,
    private val credentialState: GoogleCredentialState? = null,
    private val protectionPrompt: AuthProtectionPrompt? = null,
) : ViewModel() {

    private val _uiState = MutableStateFlow(AuthUiState())
    val uiState = _uiState.asStateFlow()

    private val _navigation = MutableSharedFlow<AuthNavigation>(extraBufferCapacity = 1)
    val navigation = _navigation.asSharedFlow()

    private var refreshGeneration = 0
    private var resendJob: Job? = null
    private var resumeAfterFreshProof: AuthStep? = null

    internal var ioDispatcher: CoroutineDispatcher = Dispatchers.IO

    private suspend fun <T> onIo(block: suspend () -> T): T = withContext(ioDispatcher) { block() }

    init {
        refreshAccount()
    }

    /** Reconcile shared storage when a retained account destination resumes. */
    fun refreshAccount() {
        if (_uiState.value.step !in setOf(AuthStep.CHECKING, AuthStep.ENTRY, AuthStep.AUTHENTICATED, AuthStep.OFFLINE_ACCOUNT)) {
            return
        }
        val generation = ++refreshGeneration
        viewModelScope.launch {
            val state = onIo {
                when (val auth = authFlow.rehydrate()) {
                    is AuthFlow.AuthState.Active -> signedInState()
                    is AuthFlow.AuthState.PendingKeyAccount -> AuthUiState(
                        step = AuthStep.KEY_ISSUED,
                        issuedUsername = auth.backup.username,
                        issuedKey = auth.backup.accountKey,
                    )
                    is AuthFlow.AuthState.NeedsRefresh -> when (val result = authFlow.refreshSession()) {
                        is AuthApiResult.Ok -> if (result.value == null) {
                            AuthUiState(step = AuthStep.ENTRY)
                        } else {
                            signedInState()
                        }
                        is AuthApiResult.Err -> retainedState(result.verdict)
                    }
                    AuthFlow.AuthState.LoggedOut -> AuthUiState(step = AuthStep.ENTRY)
                }
            }
            if (generation == refreshGeneration) {
                if (state.step == AuthStep.ENTRY) AccountKeyUnlockSession.grant.reset()
                _uiState.value = state
            }
        }
    }

    private fun signedInState(
        profile: PortableAuth.Profile? = (authFlow.profile() as? AuthApiResult.Ok)?.value,
    ): AuthUiState = AuthUiState(
        step = AuthStep.AUTHENTICATED,
        keyBackup = authFlow.currentKey(),
        profile = profile,
        showProtectionPrompt = profile != null && profile.hasAccountKey &&
            (profile.name == null || profile.recoveryEmailHint == null) &&
            protectionPrompt?.isDismissed(profile.accountId) != true,
        canReauthenticateWithGoogle = profile?.providers?.contains(PortableAuth.PROVIDER_GOOGLE) == true,
    )

    private fun retainedState(verdict: String): AuthUiState {
        val error = mapError(verdict)
        return when (authFlow.rehydrate()) {
            is AuthFlow.AuthState.Active -> signedInState(_uiState.value.profile).copy(error = error)
            is AuthFlow.AuthState.NeedsRefresh -> AuthUiState(
                step = AuthStep.OFFLINE_ACCOUNT,
                keyBackup = authFlow.currentKey(),
                error = error,
            )
            else -> AuthUiState(step = AuthStep.ENTRY, error = error)
        }
    }

    fun onGoToKeyEntry() = openEntry(AuthStep.KEY_ENTRY)
    fun beginSignUp() = openEntry(AuthStep.SIGNUP_FORM, CodePurpose.SIGNUP)
    fun beginRecovery() = openEntry(AuthStep.RECOVERY_FORM, CodePurpose.RECOVERY)

    private fun openEntry(step: AuthStep, purpose: CodePurpose = CodePurpose.SIGNUP) {
        if (_uiState.value.step != AuthStep.ENTRY) return
        refreshGeneration++
        resetResend()
        _uiState.value = AuthUiState(step = step, purpose = purpose)
    }

    fun onBackToEntry() {
        if (_uiState.value.step == AuthStep.BUSY) return
        refreshGeneration++
        resetResend()
        resumeAfterFreshProof = null
        _uiState.value = AuthUiState(step = AuthStep.ENTRY)
    }

    fun onNameChange(value: String) {
        _uiState.update { it.copy(name = value.take(256), error = null) }
    }

    fun onEmailChange(value: String) {
        _uiState.update { it.copy(email = value.take(254), error = null) }
    }

    fun onKeyChange(value: String) {
        _uiState.update { it.copy(accountKey = value.take(96), error = null) }
    }

    fun onCodeChange(value: String) {
        _uiState.update { it.copy(code = value.filter { digit -> digit in '0'..'9' }.take(6), error = null) }
    }

    fun onRequestCode(locale: String) {
        val state = _uiState.value
        if (state.step !in codeFormSteps || (state.step == AuthStep.CODE_SENT && state.resendSeconds != 0)) return
        if (state.purpose == CodePurpose.SIGNUP && !PublicName.isValid(state.name)) {
            _uiState.update { it.copy(error = AuthUiError.InvalidName) }
            return
        }
        beginBusy(state)
        viewModelScope.launch {
            when (val result = onIo { authFlow.requestCode(state.email, state.purpose, locale) }) {
                is AuthApiResult.Ok -> {
                    _uiState.value = state.copy(step = AuthStep.CODE_SENT, resendSeconds = 60, code = "", error = null)
                    startResend()
                }
                is AuthApiResult.Err -> _uiState.value = state.copy(error = mapError(result.verdict))
            }
        }
    }

    private fun startResend() {
        resendJob?.cancel()
        resendJob = viewModelScope.launch {
            repeat(60) {
                delay(1000L)
                _uiState.update { state -> state.copy(resendSeconds = (state.resendSeconds - 1).coerceAtLeast(0)) }
            }
        }
    }

    private fun resetResend() {
        resendJob?.cancel()
        resendJob = null
    }

    fun onChangeCodeEmail() {
        if (_uiState.value.step != AuthStep.CODE_SENT) return
        resetResend()
        _uiState.update {
            val step = when (it.purpose) {
                CodePurpose.SIGNUP -> AuthStep.SIGNUP_FORM
                CodePurpose.RECOVERY -> AuthStep.RECOVERY_FORM
                CodePurpose.ATTACH -> AuthStep.ATTACH_EMAIL
            }
            it.copy(step = step, code = "", resendSeconds = 0, error = null)
        }
    }

    fun onConsumeCode() {
        val state = _uiState.value
        if (state.step != AuthStep.CODE_SENT) return
        beginBusy(state)
        viewModelScope.launch {
            if (state.purpose == CodePurpose.ATTACH) {
                when (val result = onIo { authFlow.setRecoveryEmail(state.email, state.code) }) {
                    is AuthApiResult.Ok -> {
                        resetResend()
                        _uiState.value = onIo { signedInState(result.value) }
                    }
                    is AuthApiResult.Err -> restoreOrReauthenticate(state, result.verdict)
                }
            } else {
                val result = onIo {
                    if (state.purpose == CodePurpose.SIGNUP) authFlow.signUp(state.name, state.email, state.code)
                    else authFlow.recover(state.email, state.code)
                }
                when (result) {
                    is AuthApiResult.Ok -> {
                        resetResend()
                        val issued = result.value
                        _uiState.value = AuthUiState(
                            step = AuthStep.KEY_ISSUED,
                            purpose = state.purpose,
                            issuedUsername = issued.backup.username,
                            issuedKey = issued.backup.accountKey,
                            issuedKeyCanBeRecovered = true,
                            keyBackup = issued.backup.takeIf { issued.sessionSaved },
                            error = AuthUiError.SecureStorage.takeUnless { issued.sessionSaved },
                        )
                    }
                    is AuthApiResult.Err -> restore(state, mapError(result.verdict))
                }
            }
        }
    }

    fun onLoginWithKey() {
        val state = _uiState.value
        if (state.step !in setOf(AuthStep.KEY_ENTRY, AuthStep.KEY_ISSUED, AuthStep.REAUTHENTICATION)) return
        AccountKeyUnlockSession.grant.reset()
        beginBusy(state)
        viewModelScope.launch {
            val result = onIo {
                when (state.step) {
                    AuthStep.REAUTHENTICATION -> {
                        val accountId = state.profile?.accountId ?: currentAccountId()
                        if (accountId == null && resumeAfterFreshProof != null) {
                            AuthApiResult.Err(PortableAuth.Verdict.SESSION_EXPIRED)
                        } else authFlow.loginWithKey(state.accountKey, accountId)
                    }
                    AuthStep.KEY_ISSUED -> {
                        val session = authFlow.currentSession()
                        if (session == null || state.keyBackup == null) authFlow.loginWithKey(state.issuedKey)
                        else AuthApiResult.Ok(AuthFlow.Login(session, true))
                    }
                    else -> authFlow.loginWithKey(state.accountKey)
                }
            }
            when (result) {
                is AuthApiResult.Ok -> finishSignIn(state)
                is AuthApiResult.Err -> restore(state.copy(accountKey = ""), mapError(result.verdict))
            }
        }
    }

    fun onGoogle(gateway: GoogleSignInGateway) {
        val state = _uiState.value
        if (state.step !in setOf(AuthStep.ENTRY, AuthStep.REAUTHENTICATION, AuthStep.LINK_GOOGLE)) return
        if (state.step == AuthStep.REAUTHENTICATION && !state.canReauthenticateWithGoogle) return
        AccountKeyUnlockSession.grant.reset()
        beginBusy(state)
        viewModelScope.launch {
            val nonce = GoogleNonce.generate()
            when (val credential = gateway.request(nonce.claim)) {
                GoogleSignInResult.Cancelled -> restore(state, null)
                GoogleSignInResult.Unavailable -> restore(state, AuthUiError.GoogleUnavailable)
                is GoogleSignInResult.Selected -> {
                    if (state.step == AuthStep.LINK_GOOGLE) {
                        linkGoogle(state, credential.idToken, nonce.raw)
                    } else {
                        loginWithGoogle(state, credential, nonce.raw)
                    }
                }
            }
        }
    }

    private suspend fun loginWithGoogle(
        state: AuthUiState,
        credential: GoogleSignInResult.Selected,
        rawNonce: String,
    ) {
        val result = onIo {
            val expectedAccount = if (state.step == AuthStep.REAUTHENTICATION) {
                state.profile?.accountId ?: currentAccountId()
            } else null
            if (expectedAccount == null && resumeAfterFreshProof != null) {
                return@onIo AuthApiResult.Err(PortableAuth.Verdict.SESSION_EXPIRED)
            }
            authFlow.loginWithGoogle(credential.idToken, rawNonce, expectedAccount)
        }
        when (result) {
            is AuthApiResult.Err -> restore(state, mapError(result.verdict))
            is AuthApiResult.Ok -> {
                if (!result.value.profileComplete && resumeAfterFreshProof == null) {
                    _uiState.value = AuthUiState(step = AuthStep.CONFIRM_NAME, name = credential.displayName.orEmpty().take(256))
                } else finishSignIn(state)
            }
        }
    }

    private suspend fun linkGoogle(state: AuthUiState, idToken: String, rawNonce: String) {
        when (val result = onIo { authFlow.linkGoogle(idToken, rawNonce) }) {
            is AuthApiResult.Ok -> _uiState.value = onIo { signedInState() }
            is AuthApiResult.Err -> restoreOrReauthenticate(state, result.verdict)
        }
    }

    private fun currentAccountId(): String? = when (val state = authFlow.rehydrate()) {
        is AuthFlow.AuthState.Active -> state.session.accountId
        is AuthFlow.AuthState.NeedsRefresh -> state.session.accountId
        else -> null
    }

    private suspend fun finishSignIn(state: AuthUiState) {
        val resume = resumeAfterFreshProof
        resumeAfterFreshProof = null
        if (state.step == AuthStep.REAUTHENTICATION && resume != null) {
            restore(state.copy(step = resume, accountKey = ""), null)
            return
        }
        _uiState.value = onIo { signedInState() }
        _navigation.emit(AuthNavigation.NavigateBack)
    }

    /** Company access uses the same explicit proof without switching accounts. */
    fun beginReauthentication() {
        val state = _uiState.value
        if (state.step == AuthStep.BUSY) return
        AccountKeyUnlockSession.grant.reset()
        enterReauthentication(state, null)
        val generation = refreshGeneration
        viewModelScope.launch {
            val (profile, canSignInWithGoogle) = onIo {
                val accountId = currentAccountId()
                val profile = (authFlow.profile() as? AuthApiResult.Ok)?.value
                val canSignInWithGoogle = accountId == null || PortableAuth.PROVIDER_GOOGLE in profile?.providers.orEmpty()
                profile to canSignInWithGoogle
            }
            if (generation == refreshGeneration && _uiState.value.step == AuthStep.REAUTHENTICATION) {
                _uiState.update { it.copy(profile = profile, canReauthenticateWithGoogle = canSignInWithGoogle) }
            }
        }
    }

    fun beginRecoveryEmailChange() {
        val state = _uiState.value
        if (state.step != AuthStep.AUTHENTICATED) return
        enterReauthentication(state.copy(purpose = CodePurpose.ATTACH), AuthStep.ATTACH_EMAIL)
    }

    fun beginGoogleLink() {
        val state = _uiState.value
        if (state.step != AuthStep.AUTHENTICATED) return
        enterReauthentication(state, AuthStep.LINK_GOOGLE)
    }

    private fun enterReauthentication(state: AuthUiState, resume: AuthStep?, error: AuthUiError? = null) {
        refreshGeneration++
        resumeAfterFreshProof = resume
        _uiState.value = state.copy(step = AuthStep.REAUTHENTICATION, accountKey = "", error = error)
    }

    private fun restoreOrReauthenticate(state: AuthUiState, verdict: String) {
        val error = mapError(verdict)
        if (error == AuthUiError.ReauthenticationRequired) {
            enterReauthentication(state.copy(resendSeconds = _uiState.value.resendSeconds), state.step, error)
        } else restore(state, error)
    }

    fun resumePublicNameConfirmation() {
        val state = _uiState.value
        val profile = state.profile ?: return
        if (state.step == AuthStep.AUTHENTICATED && !profile.profileComplete &&
            PortableAuth.PROVIDER_GOOGLE in profile.providers
        ) {
            _uiState.value = state.copy(step = AuthStep.CONFIRM_NAME)
        }
    }

    fun beginPublicNameEdit() {
        val state = _uiState.value
        if (state.step != AuthStep.AUTHENTICATED) return
        refreshGeneration++
        _uiState.value = state.copy(step = AuthStep.EDIT_NAME, name = state.profile?.name.orEmpty(), error = null)
    }

    fun onSaveName() {
        val state = _uiState.value
        if (state.step !in setOf(AuthStep.CONFIRM_NAME, AuthStep.EDIT_NAME)) return
        if (!PublicName.isValid(state.name)) {
            _uiState.update { it.copy(error = AuthUiError.InvalidName) }
            return
        }
        beginBusy(state)
        viewModelScope.launch {
            when (val result = onIo { authFlow.updateName(state.name) }) {
                is AuthApiResult.Ok -> {
                    _uiState.value = onIo { signedInState(result.value) }
                    if (state.step == AuthStep.CONFIRM_NAME) _navigation.emit(AuthNavigation.NavigateBack)
                }
                is AuthApiResult.Err -> restore(state, mapError(result.verdict))
            }
        }
    }

    fun onCancelProfileEdit() {
        if (_uiState.value.step == AuthStep.BUSY) return
        refreshGeneration++
        resumeAfterFreshProof = null
        resetResend()
        _uiState.update {
            it.copy(
                step = AuthStep.AUTHENTICATED,
                email = "",
                code = "",
                name = "",
                accountKey = "",
                purpose = CodePurpose.SIGNUP,
                resendSeconds = 0,
                error = null,
            )
        }
    }

    fun dismissProtectionPrompt() {
        val accountId = _uiState.value.profile?.accountId ?: return
        protectionPrompt?.dismiss(accountId)
        _uiState.update { it.copy(showProtectionPrompt = false) }
    }

    fun onLogout() {
        if (_uiState.value.step == AuthStep.BUSY) return
        AccountKeyUnlockSession.grant.reset()
        beginBusy(_uiState.value)
        viewModelScope.launch {
            onIo { ownerErasure.purgeCurrentOwner() }
            val result = onIo { authFlow.logout() }
            credentialState?.clear()
            resetResend()
            resumeAfterFreshProof = null
            _uiState.value = if (result is AuthApiResult.Err && result.verdict == AuthFlow.SECURE_STORAGE_UNAVAILABLE) {
                onIo { retainedState(result.verdict) }
            } else AuthUiState(step = AuthStep.ENTRY)
        }
    }

    fun onDeleteAccount() {
        if (_uiState.value.step == AuthStep.BUSY) return
        AccountKeyUnlockSession.grant.reset()
        val snapshot = _uiState.value
        beginBusy(snapshot)
        viewModelScope.launch {
            val scope = onIo { ownerErasure.currentScopeOrNull() }
            val accountId = snapshot.profile?.accountId ?: onIo { currentAccountId() }
            when (val result = onIo { authFlow.deleteAccount() }) {
                is AuthApiResult.Ok -> {
                    if (scope != null) onIo { ownerErasure.purgeScope(scope) }
                    accountId?.let { protectionPrompt?.clear(it) }
                    credentialState?.clear()
                    resetResend()
                    resumeAfterFreshProof = null
                    _uiState.value = AuthUiState(step = AuthStep.ENTRY, error = AuthUiError.Deleted)
                }
                is AuthApiResult.Err -> {
                    val retained = onIo { retainedState(result.verdict) }
                    if (retained.step == AuthStep.ENTRY) {
                        if (scope != null) onIo { ownerErasure.purgeScope(scope) }
                        credentialState?.clear()
                    }
                    _uiState.value = retained
                }
            }
        }
    }

    private fun beginBusy(state: AuthUiState) {
        refreshGeneration++
        _uiState.value = state.copy(step = AuthStep.BUSY, busyOrigin = state.step, error = null)
    }

    private fun restore(state: AuthUiState, error: AuthUiError?) {
        _uiState.value = state.copy(error = error, resendSeconds = _uiState.value.resendSeconds)
    }

    fun onDismissError() {
        _uiState.update { it.copy(error = null) }
    }

    companion object {
        private val codeFormSteps = setOf(AuthStep.SIGNUP_FORM, AuthStep.RECOVERY_FORM, AuthStep.ATTACH_EMAIL, AuthStep.CODE_SENT)

        fun mapError(verdict: String): AuthUiError = when (verdict) {
            AuthFlow.CLIENT_INVALID -> AuthUiError.InvalidInput
            AuthFlow.SECURE_STORAGE_UNAVAILABLE -> AuthUiError.SecureStorage
            PortableAuth.Verdict.CODE_UNKNOWN, PortableAuth.Verdict.CODE_CONSUMED -> AuthUiError.WrongCode
            "key-invalid" -> AuthUiError.WrongKey
            "username-invalid" -> AuthUiError.InvalidInput
            PortableAuth.Verdict.CODE_EXPIRED -> AuthUiError.ExpiredCode
            PortableAuth.Verdict.CODE_LOCKED -> AuthUiError.LockedCode
            PortableAuth.Verdict.SESSION_REUSE, PortableAuth.Verdict.SESSION_REVOKED,
            PortableAuth.Verdict.SESSION_INVALID, PortableAuth.Verdict.SESSION_EXPIRED -> AuthUiError.SessionExpired
            PortableAuth.Verdict.ACCOUNT_SUSPENDED -> AuthUiError.Suspended
            PortableAuth.Verdict.ACCOUNT_DELETED -> AuthUiError.Deleted
            PortableAuth.Verdict.NAME_INVALID -> AuthUiError.InvalidName
            PortableAuth.Verdict.ADDRESS_LINKED -> AuthUiError.AddressLinked
            PortableAuth.Verdict.RECOVERY_NO_ACCOUNT -> AuthUiError.RecoveryNoAccount
            PortableAuth.Verdict.PROVIDER_EMAIL_IN_USE -> AuthUiError.ProviderEmailInUse
            PortableAuth.Verdict.REAUTHENTICATION_REQUIRED -> AuthUiError.ReauthenticationRequired
            PortableAuth.Verdict.EMAIL_LOGIN_RETIRED -> AuthUiError.EmailLoginRetired
            PortableAuth.Verdict.PROVIDER_UNAVAILABLE, "oidc-unavailable" -> AuthUiError.GoogleUnavailable
            PortableAuth.UNAVAILABLE -> AuthUiError.Offline
            else -> if (verdict.startsWith("oidc-") || verdict.startsWith("link-")) AuthUiError.ProviderDenied
                else AuthUiError.Unknown
        }
    }
}
