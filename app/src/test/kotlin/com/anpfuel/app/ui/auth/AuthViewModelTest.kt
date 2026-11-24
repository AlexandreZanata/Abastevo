package com.anpfuel.app.ui.auth

import app.cash.turbine.test
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.GoogleLogin
import com.anpfuel.application.portable.GoogleSignInGateway
import com.anpfuel.application.portable.GoogleSignInResult
import com.anpfuel.domain.account.GoogleNonce
import com.anpfuel.domain.portable.PortableAuth.CodePurpose
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.data.repository.OwnerLocalErasure
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class AuthViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private val authFlow = mockk<AuthFlow>()
    private val ownerErasure = mockk<OwnerLocalErasure>(relaxed = true)
    private lateinit var viewModel: AuthViewModel

    private fun session() = PortableAuth.Session(
        familyId = "family-1", accountId = "account-1",
        accessToken = "access-1", refreshToken = "refresh-1",
        accessExpiresAt = 1_000_900L, absoluteExpiresAt = 4_259_200L,
    )

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        every { authFlow.rehydrate() } returns AuthFlow.AuthState.LoggedOut
        every { authFlow.currentKey() } returns null
        every { authFlow.profile() } returns AuthApiResult.Err(PortableAuth.UNAVAILABLE)
        viewModel = AuthViewModel(authFlow, ownerErasure).also { it.ioDispatcher = dispatcher }
        viewModel.ioDispatcher = dispatcher
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    private val signupBackup = AuthFlow.KeyBackup("Sample Person", "synthetic-key")

    private fun kotlinx.coroutines.test.TestScope.setupSignUp(result: AuthApiResult<AuthFlow.NewKey>) {
        advanceUntilIdle()
        every { authFlow.requestCode("case-01@example.invalid", CodePurpose.SIGNUP, "en") } returns AuthApiResult.Ok(Unit)
        every { authFlow.signUp(signupBackup.username, "case-01@example.invalid", "482916") } returns result
        viewModel.beginSignUp()
        viewModel.onNameChange(signupBackup.username)
        viewModel.onEmailChange("case-01@example.invalid")
        viewModel.onRequestCode("en")
        advanceUntilIdle()
        viewModel.onCodeChange("482916")
    }

    @Test
    fun startsLoggedOutAtEntry() = runTest {
        advanceUntilIdle()
        val state = viewModel.uiState.value
        assertEquals(AuthStep.ENTRY, state.step)
        assertNull(state.error)
    }

    @Test
    fun rehydratesActiveSession() = runTest {
        every { authFlow.rehydrate() } returns AuthFlow.AuthState.Active(session())
        viewModel = AuthViewModel(authFlow, ownerErasure).also { it.ioDispatcher = dispatcher }
        advanceUntilIdle()
        assertEquals(AuthStep.AUTHENTICATED, viewModel.uiState.value.step)
    }

    @Test
    fun rotatesStaleSessionOnStart() = runTest {
        every { authFlow.rehydrate() } returns AuthFlow.AuthState.NeedsRefresh(session())
        every { authFlow.refreshSession() } returns AuthApiResult.Ok(session())
        viewModel = AuthViewModel(authFlow, ownerErasure).also { it.ioDispatcher = dispatcher }
        advanceUntilIdle()
        assertEquals(AuthStep.AUTHENTICATED, viewModel.uiState.value.step)
    }

    @Test
    fun deadSessionReturnsToEntry() = runTest {
        every { authFlow.rehydrate() } returns AuthFlow.AuthState.NeedsRefresh(session())
        every { authFlow.refreshSession() } returns AuthApiResult.Ok(null)
        viewModel = AuthViewModel(authFlow, ownerErasure).also { it.ioDispatcher = dispatcher }
        advanceUntilIdle()
        assertEquals(AuthStep.ENTRY, viewModel.uiState.value.step)
    }

    @Test
    fun recoveryKeyAcknowledgementNavigatesBack() = runTest {
        advanceUntilIdle()
        every { authFlow.requestCode("case-01@example.invalid", CodePurpose.RECOVERY, "en") } returns AuthApiResult.Ok(Unit)
        every { authFlow.recover("case-01@example.invalid", "482916") } returns
            AuthApiResult.Ok(AuthFlow.NewKey(AuthFlow.KeyBackup("Sample Person", "synthetic-key"), true))
        every { authFlow.currentSession() } returns session()
        viewModel.beginRecovery()
        viewModel.onEmailChange("case-01@example.invalid")
        viewModel.onRequestCode("en")
        advanceUntilIdle()
        viewModel.onCodeChange("482916")
        viewModel.navigation.test {
            viewModel.onConsumeCode()
            advanceUntilIdle()
            assertEquals(AuthStep.KEY_ISSUED, viewModel.uiState.value.step)
            expectNoEvents()
            viewModel.onLoginWithKey()
            advanceUntilIdle()
            assertEquals(AuthStep.AUTHENTICATED, viewModel.uiState.value.step)
            assertEquals(AuthNavigation.NavigateBack, awaitItem())
        }
    }

    @Test
    fun wrongCodeKeepsCodeEntryWithMessage() = runTest {
        advanceUntilIdle()
        every { authFlow.requestCode(any(), CodePurpose.RECOVERY, "en") } returns AuthApiResult.Ok(Unit)
        every { authFlow.recover(any(), any()) } returns
            AuthApiResult.Err(PortableAuth.Verdict.CODE_UNKNOWN)
        viewModel.beginRecovery()
        viewModel.onEmailChange("case-01@example.invalid")
        viewModel.onRequestCode("en")
        advanceUntilIdle()
        viewModel.onCodeChange("000000")
        viewModel.onConsumeCode()
        advanceUntilIdle()
        val state = viewModel.uiState.value
        assertEquals(AuthStep.CODE_SENT, state.step)
        assertEquals(AuthUiError.WrongCode, state.error)
    }

    @Test
    fun suspendedMapsToSuspendedMessage() = runTest {
        advanceUntilIdle()
        every { authFlow.requestCode(any(), CodePurpose.RECOVERY, "en") } returns AuthApiResult.Ok(Unit)
        every { authFlow.recover(any(), any()) } returns
            AuthApiResult.Err(PortableAuth.Verdict.ACCOUNT_SUSPENDED)
        viewModel.beginRecovery()
        viewModel.onEmailChange("case-01@example.invalid")
        viewModel.onRequestCode("en")
        advanceUntilIdle()
        viewModel.onCodeChange("482916")
        viewModel.onConsumeCode()
        advanceUntilIdle()
        assertEquals(AuthUiError.Suspended, viewModel.uiState.value.error)
    }

    @Test
    fun credentialManagerUsesHashedNonceAndApiReceivesTheBoundRawNonce() = runTest {
        advanceUntilIdle()
        var claim = ""
        every { authFlow.loginWithGoogle("synthetic-id-token", any()) } answers {
            assertEquals(claim, GoogleNonce.claimFor(secondArg<String>()))
            AuthApiResult.Ok(GoogleLogin(session(), created = false, profileComplete = true))
        }
        viewModel.onGoogle(GoogleSignInGateway {
            claim = it
            GoogleSignInResult.Selected("synthetic-id-token", null)
        })
        advanceUntilIdle()
        assertEquals(AuthStep.AUTHENTICATED, viewModel.uiState.value.step)
        assertNull(viewModel.uiState.value.error)
    }

    @Test
    fun unavailableProviderCredentialDoesNotSendAnApiRequest() = runTest {
        advanceUntilIdle()
        viewModel.onGoogle(GoogleSignInGateway { GoogleSignInResult.Unavailable })
        advanceUntilIdle()
        assertEquals(AuthStep.ENTRY, viewModel.uiState.value.step)
        assertEquals(AuthUiError.GoogleUnavailable, viewModel.uiState.value.error)
        verify(exactly = 0) { authFlow.loginWithGoogle(any(), any()) }
    }

    @Test
    fun logoutReturnsToEntry() = runTest {
        advanceUntilIdle()
        every { authFlow.logout() } returns AuthApiResult.Ok(Unit)
        viewModel.onLogout()
        advanceUntilIdle()
        val state = viewModel.uiState.value
        assertEquals(AuthStep.ENTRY, state.step)
        assertEquals("", state.email)
    }

    // An offline revoke after a durable local clear is still a completed local logout.
    @Test
    fun offlineRevokeAfterDurableClearStillLogsOut() = runTest {
        advanceUntilIdle()
        every { authFlow.logout() } returns AuthApiResult.Err(PortableAuth.UNAVAILABLE)
        viewModel.onLogout()
        advanceUntilIdle()
        val state = viewModel.uiState.value
        assertEquals(AuthStep.ENTRY, state.step)
        assertNull(state.error)
    }

    // A failed durable clear is never shown as a completed logout.
    @Test
    fun failedDurableClearReportsSecureStorageAndKeepsRetainedSession() = runTest {
        advanceUntilIdle()
        every { authFlow.logout() } returns AuthApiResult.Err(AuthFlow.SECURE_STORAGE_UNAVAILABLE)
        every { authFlow.rehydrate() } returns AuthFlow.AuthState.Active(session())
        viewModel.onLogout()
        advanceUntilIdle()
        val state = viewModel.uiState.value
        assertEquals(AuthStep.AUTHENTICATED, state.step)
        assertEquals(AuthUiError.SecureStorage, state.error)
    }

    @Test
    fun failedDurableClearWithoutRetainedSessionStillReportsSecureStorage() = runTest {
        advanceUntilIdle()
        every { authFlow.logout() } returns AuthApiResult.Err(AuthFlow.SECURE_STORAGE_UNAVAILABLE)
        viewModel.onLogout()
        advanceUntilIdle()
        val state = viewModel.uiState.value
        assertEquals(AuthStep.ENTRY, state.step)
        assertEquals(AuthUiError.SecureStorage, state.error)
    }

    @Test
    fun deleteAccountReturnsToEntryWithNotice() = runTest {
        advanceUntilIdle()
        every { authFlow.deleteAccount() } returns AuthApiResult.Ok(Unit)
        viewModel.onDeleteAccount()
        advanceUntilIdle()
        val state = viewModel.uiState.value
        assertEquals(AuthStep.ENTRY, state.step)
        assertEquals(AuthUiError.Deleted, state.error)
        verify(exactly = 1) { authFlow.deleteAccount() }
    }

    @Test
    fun deniedDeletionClearsDisplayedAuthority() = runTest {
        advanceUntilIdle()
        every { authFlow.deleteAccount() } returns
            AuthApiResult.Err(PortableAuth.Verdict.ACCOUNT_SUSPENDED)
        viewModel.onDeleteAccount()
        advanceUntilIdle()
        assertEquals(AuthUiError.Suspended, viewModel.uiState.value.error)
        assertEquals(AuthStep.ENTRY, viewModel.uiState.value.step)
        assertNull(viewModel.uiState.value.keyBackup)
    }

    @Test
    fun mapsProviderVerdicts() {
        assertEquals(
            AuthUiError.ProviderDenied,
            AuthViewModel.mapError("oidc-nonce-reused"),
        )
        assertEquals(
            AuthUiError.ProviderDenied,
            AuthViewModel.mapError("link-cross-account-refused"),
        )
        assertEquals(AuthUiError.Offline, AuthViewModel.mapError(PortableAuth.UNAVAILABLE))
        assertEquals(AuthUiError.Unknown, AuthViewModel.mapError("something-new"))
        assertEquals(AuthUiError.WrongKey, AuthViewModel.mapError("key-invalid"))
        assertEquals(AuthUiError.InvalidName, AuthViewModel.mapError("name-invalid"))
        assertEquals(AuthUiError.AddressLinked, AuthViewModel.mapError("address-linked"))
        assertEquals(AuthUiError.RecoveryNoAccount, AuthViewModel.mapError("recovery-no-account"))
        assertEquals(AuthUiError.ProviderEmailInUse, AuthViewModel.mapError("provider-email-in-use"))
        assertEquals(AuthUiError.ReauthenticationRequired, AuthViewModel.mapError("reauthentication-required"))
        assertEquals(AuthUiError.EmailLoginRetired, AuthViewModel.mapError("email-login-retired"))
    }

    @Test
    fun signupSignsInAndKeepsBackupStepVisible() = runTest {
        setupSignUp(AuthApiResult.Ok(AuthFlow.NewKey(signupBackup, true)))
        viewModel.onConsumeCode()
        advanceUntilIdle()
        val state = viewModel.uiState.value
        assertEquals(AuthStep.KEY_ISSUED, state.step)
        assertEquals(signupBackup.username, state.issuedUsername)
        assertEquals(signupBackup.accountKey, state.issuedKey)
        verify(exactly = 0) { authFlow.loginWithKey(any()) }
    }

    @Test
    fun continueAfterSignupDoesNotIssueAnotherSession() = runTest {
        setupSignUp(AuthApiResult.Ok(AuthFlow.NewKey(signupBackup, true)))
        every { authFlow.currentSession() } returns session()
        viewModel.onConsumeCode()
        advanceUntilIdle()
        viewModel.navigation.test {
            viewModel.onLoginWithKey()
            advanceUntilIdle()
            assertEquals(AuthStep.AUTHENTICATED, viewModel.uiState.value.step)
            assertEquals("", viewModel.uiState.value.issuedKey)
            assertEquals(AuthNavigation.NavigateBack, awaitItem())
        }
        verify(exactly = 0) { authFlow.loginWithKey(any()) }
    }

    @Test
    fun signupStorageFailureKeepsRecoveryKeyAndDoesNotNavigate() = runTest {
        setupSignUp(AuthApiResult.Ok(AuthFlow.NewKey(signupBackup, false)))
        every { authFlow.loginWithKey(signupBackup.accountKey) } returns AuthApiResult.Err(AuthFlow.SECURE_STORAGE_UNAVAILABLE)
        every { authFlow.currentSession() } returns null
        viewModel.navigation.test {
            viewModel.onConsumeCode()
            advanceUntilIdle()
            assertEquals(AuthStep.KEY_ISSUED, viewModel.uiState.value.step)
            assertEquals(AuthUiError.SecureStorage, viewModel.uiState.value.error)
            assertEquals(signupBackup.accountKey, viewModel.uiState.value.issuedKey)
            expectNoEvents()
            viewModel.onLoginWithKey()
            advanceUntilIdle()
            assertEquals(AuthStep.KEY_ISSUED, viewModel.uiState.value.step)
            assertEquals(signupBackup.accountKey, viewModel.uiState.value.issuedKey)
            expectNoEvents()
        }
    }

    @Test
    fun linkedRecoveryEmailKeepsSignupCodeStepWithError() = runTest {
        setupSignUp(AuthApiResult.Err("address-linked"))
        viewModel.onConsumeCode()
        advanceUntilIdle()
        val state = viewModel.uiState.value
        assertEquals(AuthStep.CODE_SENT, state.step)
        assertEquals(AuthUiError.AddressLinked, state.error)
    }

    @Test
    fun keyLoginNavigatesBack() = runTest {
        advanceUntilIdle()
        every { authFlow.loginWithKey("key-for-ana123") } returns
            AuthApiResult.Ok(AuthFlow.Login(session(), created = false))
        every { authFlow.currentKey() } returns AuthFlow.KeyBackup("ana123", "key-for-ana123")
        viewModel.onGoToKeyEntry()
        viewModel.onKeyChange("key-for-ana123")
        viewModel.navigation.test {
            viewModel.onLoginWithKey()
            advanceUntilIdle()
            val state = viewModel.uiState.value
            assertEquals(AuthStep.AUTHENTICATED, state.step)
            assertEquals("ana123", state.keyBackup?.username)
            assertEquals(AuthNavigation.NavigateBack, awaitItem())
        }
    }

    @Test
    fun wrongKeyMapsError() = runTest {
        advanceUntilIdle()
        every { authFlow.loginWithKey(any()) } returns AuthApiResult.Err("key-invalid")
        viewModel.onGoToKeyEntry()
        viewModel.onKeyChange("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
        viewModel.onLoginWithKey()
        advanceUntilIdle()
        val state = viewModel.uiState.value
        assertEquals(AuthStep.KEY_ENTRY, state.step)
        assertEquals(AuthUiError.WrongKey, state.error)
    }
    @Test
    fun offlineRefreshKeepsLocalAccountInsteadOfSignup() = runTest {
        every { authFlow.rehydrate() } returns AuthFlow.AuthState.NeedsRefresh(session())
        every { authFlow.refreshSession() } returns AuthApiResult.Err(PortableAuth.UNAVAILABLE)
        every { authFlow.currentKey() } returns AuthFlow.KeyBackup("ana123", "key-for-ana123")
        viewModel = AuthViewModel(authFlow, ownerErasure).also { it.ioDispatcher = dispatcher }
        advanceUntilIdle()
        assertEquals(AuthStep.OFFLINE_ACCOUNT, viewModel.uiState.value.step)
        assertEquals(AuthUiError.Offline, viewModel.uiState.value.error)
        assertEquals("ana123", viewModel.uiState.value.keyBackup?.username)
    }

    @Test
    fun signupLoginOutageKeepsIssuedKeyForRetry() = runTest {
        setupSignUp(AuthApiResult.Ok(AuthFlow.NewKey(signupBackup, false)))
        every { authFlow.loginWithKey(signupBackup.accountKey) } returns AuthApiResult.Err(PortableAuth.UNAVAILABLE)
        every { authFlow.currentSession() } returns null
        viewModel.onConsumeCode()
        viewModel.onConsumeCode()
        advanceUntilIdle()
        assertEquals(AuthStep.KEY_ISSUED, viewModel.uiState.value.step)
        assertEquals(signupBackup.accountKey, viewModel.uiState.value.issuedKey)
        viewModel.onLoginWithKey()
        advanceUntilIdle()
        assertEquals(AuthStep.KEY_ISSUED, viewModel.uiState.value.step)
        assertEquals(signupBackup.accountKey, viewModel.uiState.value.issuedKey)
        verify(exactly = 1) { authFlow.signUp(any(), any(), any()) }
    }

    @Test
    fun resumeReflectsLogoutFromAnotherDestination() = runTest {
        every { authFlow.rehydrate() } returns AuthFlow.AuthState.Active(session())
        viewModel = AuthViewModel(authFlow, ownerErasure).also { it.ioDispatcher = dispatcher }
        advanceUntilIdle()
        every { authFlow.rehydrate() } returns AuthFlow.AuthState.LoggedOut
        viewModel.refreshAccount()
        advanceUntilIdle()
        assertEquals(AuthStep.ENTRY, viewModel.uiState.value.step)
        assertNull(viewModel.uiState.value.keyBackup)
    }

    @Test
    fun deletionOutageKeepsSignedInStateAndBackup() = runTest {
        val backup = AuthFlow.KeyBackup("ana123", "key-for-ana123")
        every { authFlow.rehydrate() } returns AuthFlow.AuthState.Active(session())
        every { authFlow.currentKey() } returns backup
        every { authFlow.deleteAccount() } returns AuthApiResult.Err(PortableAuth.UNAVAILABLE)
        viewModel = AuthViewModel(authFlow, ownerErasure).also { it.ioDispatcher = dispatcher }
        advanceUntilIdle()
        viewModel.onDeleteAccount()
        advanceUntilIdle()
        assertEquals(AuthStep.AUTHENTICATED, viewModel.uiState.value.step)
        assertEquals(backup, viewModel.uiState.value.keyBackup)
        assertEquals(AuthUiError.Offline, viewModel.uiState.value.error)
    }

    // SEC04-D: logout purges the owner's queue and private media copies
    // before identity clears; deletion purges only when local
    // authority is gone, never on transport failure.
    @Test
    fun logoutPurgesOwnerQueue() = runTest {
        advanceUntilIdle()
        every { authFlow.logout() } returns AuthApiResult.Ok(Unit)
        viewModel.onLogout()
        advanceUntilIdle()
        coVerify(exactly = 1) { ownerErasure.purgeCurrentOwner() }
    }

    @Test
    fun deleteAccountPurgesCapturedScope() = runTest {
        advanceUntilIdle()
        coEvery { ownerErasure.currentScopeOrNull() } returns "scope-a"
        coEvery { ownerErasure.purgeScope("scope-a") } returns OwnerLocalErasure.Report(2, 1, 2)
        every { authFlow.deleteAccount() } returns AuthApiResult.Ok(Unit)
        viewModel.onDeleteAccount()
        advanceUntilIdle()
        coVerify(exactly = 1) { ownerErasure.purgeScope("scope-a") }
    }

    @Test
    fun failedDeletionKeepsQueue() = runTest {
        advanceUntilIdle()
        coEvery { ownerErasure.currentScopeOrNull() } returns "scope-a"
        every { authFlow.deleteAccount() } returns AuthApiResult.Err(PortableAuth.UNAVAILABLE)
        every { authFlow.rehydrate() } returns AuthFlow.AuthState.Active(
            PortableAuth.Session("family-1", "account-1", "access-1", "refresh-1", 1_000_900L, 4_259_200L),
        )
        viewModel.onDeleteAccount()
        advanceUntilIdle()
        coVerify(exactly = 0) { ownerErasure.purgeScope(any()) }
    }
}
