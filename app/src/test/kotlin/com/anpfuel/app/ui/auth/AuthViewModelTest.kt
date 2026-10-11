package com.anpfuel.app.ui.auth

import app.cash.turbine.test
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.ConsumeOk
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
import org.junit.jupiter.api.Assertions.assertTrue
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
        viewModel = AuthViewModel(authFlow, ownerErasure).also { it.ioDispatcher = dispatcher }
        viewModel.ioDispatcher = dispatcher
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun startsLoggedOutAtUsernameEntry() = runTest {
        advanceUntilIdle()
        val state = viewModel.uiState.value
        assertEquals(AuthStep.USERNAME_ENTRY, state.step)
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
    fun deadSessionReturnsToEmailEntry() = runTest {
        every { authFlow.rehydrate() } returns AuthFlow.AuthState.NeedsRefresh(session())
        every { authFlow.refreshSession() } returns AuthApiResult.Ok(null)
        viewModel = AuthViewModel(authFlow, ownerErasure).also { it.ioDispatcher = dispatcher }
        advanceUntilIdle()
        assertEquals(AuthStep.USERNAME_ENTRY, viewModel.uiState.value.step)
    }

    @Test
    fun emailLoginNavigatesBack() = runTest {
        advanceUntilIdle()
        every { authFlow.requestEmailCode("case-01@example.invalid") } returns AuthApiResult.Ok(Unit)
        every { authFlow.consumeEmailCode("case-01@example.invalid", "482916") } returns
            AuthApiResult.Ok(AuthFlow.Login(session(), created = true))
        viewModel.onEmailChange("case-01@example.invalid")
        viewModel.onRequestCode()
        advanceUntilIdle()
        assertEquals(AuthStep.CODE_SENT, viewModel.uiState.value.step)
        viewModel.onCodeChange("482916")
        viewModel.navigation.test {
            viewModel.onConsumeCode()
            advanceUntilIdle()
            assertEquals(AuthStep.AUTHENTICATED, viewModel.uiState.value.step)
            assertEquals(AuthNavigation.NavigateBack, awaitItem())
        }
    }

    @Test
    fun wrongCodeKeepsCodeEntryWithMessage() = runTest {
        advanceUntilIdle()
        every { authFlow.requestEmailCode(any()) } returns AuthApiResult.Ok(Unit)
        every { authFlow.consumeEmailCode(any(), any()) } returns
            AuthApiResult.Err(PortableAuth.Verdict.CODE_UNKNOWN)
        viewModel.onEmailChange("case-01@example.invalid")
        viewModel.onRequestCode()
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
        every { authFlow.requestEmailCode(any()) } returns AuthApiResult.Ok(Unit)
        every { authFlow.consumeEmailCode(any(), any()) } returns
            AuthApiResult.Err(PortableAuth.Verdict.ACCOUNT_SUSPENDED)
        viewModel.onEmailChange("case-01@example.invalid")
        viewModel.onRequestCode()
        advanceUntilIdle()
        viewModel.onCodeChange("482916")
        viewModel.onConsumeCode()
        advanceUntilIdle()
        assertEquals(AuthUiError.Suspended, viewModel.uiState.value.error)
    }

    @Test
    fun providerAttemptAndCompletion() = runTest {
        advanceUntilIdle()
        val attempt = PortableAuth.LoginAttempt("google", "n-1", "s-1")
        every { authFlow.beginProviderLogin("google") } returns AuthApiResult.Ok(attempt)
        every {
            authFlow.completeProviderLogin(any(), any(), any())
        } returns AuthApiResult.Ok(Unit)
        every { authFlow.currentSession() } returns session()
        viewModel.onProviderClick("google")
        advanceUntilIdle()
        assertEquals(AuthStep.PROVIDER_PENDING, viewModel.uiState.value.step)
        viewModel.onProviderCallback("google", "tok", "n-1", "s-1")
        advanceUntilIdle()
        assertEquals(AuthStep.AUTHENTICATED, viewModel.uiState.value.step)
        assertTrue(viewModel.uiState.value.error == null)
    }

    @Test
    fun callbackWithoutAttemptIsIgnored() = runTest {
        advanceUntilIdle()
        viewModel.onProviderCallback("google", "tok", "n-1", "s-1")
        advanceUntilIdle()
        assertEquals(AuthUiError.InvalidInput, viewModel.uiState.value.error)
        verify(exactly = 0) { authFlow.completeProviderLogin(any(), any(), any()) }
    }

    @Test
    fun logoutReturnsToEmailEntry() = runTest {
        advanceUntilIdle()
        every { authFlow.logout() } returns AuthApiResult.Ok(Unit)
        viewModel.onLogout()
        advanceUntilIdle()
        val state = viewModel.uiState.value
        assertEquals(AuthStep.USERNAME_ENTRY, state.step)
        assertEquals("", state.email)
    }

    @Test
    fun deleteAccountReturnsToEmailEntryWithNotice() = runTest {
        advanceUntilIdle()
        every { authFlow.deleteAccount() } returns AuthApiResult.Ok(Unit)
        viewModel.onDeleteAccount()
        advanceUntilIdle()
        val state = viewModel.uiState.value
        assertEquals(AuthStep.USERNAME_ENTRY, state.step)
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
        assertEquals(AuthStep.USERNAME_ENTRY, viewModel.uiState.value.step)
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
        assertEquals(AuthUiError.UsernameTaken, AuthViewModel.mapError("username-taken"))
    }

    @Test
    fun keySignupSignsInAndKeepsBackupStepVisible() = runTest {
        advanceUntilIdle()
        every { authFlow.createKeyAccount("ana123") } returns
            AuthApiResult.Ok(AuthFlow.KeyIssued("ana123", "key-for-ana123"))
        every { authFlow.loginWithKey("key-for-ana123") } returns AuthApiResult.Ok(AuthFlow.Login(session(), true))
        viewModel.onUsernameChange("Ana123")
        viewModel.onCreateAccount()
        advanceUntilIdle()
        val state = viewModel.uiState.value
        assertEquals(AuthStep.KEY_ISSUED, state.step)
        assertEquals("ana123", state.issuedUsername)
        assertEquals("key-for-ana123", state.issuedKey)
        verify(exactly = 1) { authFlow.loginWithKey("key-for-ana123") }
    }

    @Test
    fun continueAfterAutomaticSignupLoginDoesNotIssueAnotherSession() = runTest {
        advanceUntilIdle()
        every { authFlow.createKeyAccount("ana123") } returns AuthApiResult.Ok(AuthFlow.KeyIssued("ana123", "key-for-ana123"))
        every { authFlow.loginWithKey("key-for-ana123") } returns AuthApiResult.Ok(AuthFlow.Login(session(), true))
        every { authFlow.currentSession() } returns session()
        viewModel.onUsernameChange("ana123")
        viewModel.onCreateAccount()
        advanceUntilIdle()
        viewModel.navigation.test {
            viewModel.onLoginWithKey()
            advanceUntilIdle()
            assertEquals(AuthStep.AUTHENTICATED, viewModel.uiState.value.step)
            assertEquals("", viewModel.uiState.value.issuedKey)
            assertEquals(AuthNavigation.NavigateBack, awaitItem())
        }
        verify(exactly = 1) { authFlow.loginWithKey("key-for-ana123") }
    }

    @Test
    fun signupStorageFailureKeepsRecoveryKeyAndDoesNotNavigate() = runTest {
        advanceUntilIdle()
        every { authFlow.createKeyAccount("ana123") } returns AuthApiResult.Ok(AuthFlow.KeyIssued("ana123", "key-for-ana123"))
        every { authFlow.loginWithKey("key-for-ana123") } returns AuthApiResult.Err(AuthFlow.SECURE_STORAGE_UNAVAILABLE)
        every { authFlow.currentSession() } returns null
        viewModel.onUsernameChange("ana123")
        viewModel.navigation.test {
            viewModel.onCreateAccount()
            advanceUntilIdle()
            assertEquals(AuthStep.KEY_ISSUED, viewModel.uiState.value.step)
            assertEquals(AuthUiError.SecureStorage, viewModel.uiState.value.error)
            assertEquals("key-for-ana123", viewModel.uiState.value.issuedKey)
            expectNoEvents()
            viewModel.onLoginWithKey()
            advanceUntilIdle()
            assertEquals(AuthStep.KEY_ISSUED, viewModel.uiState.value.step)
            assertEquals("key-for-ana123", viewModel.uiState.value.issuedKey)
            expectNoEvents()
        }
    }

    @Test
    fun takenUsernameMapsError() = runTest {
        advanceUntilIdle()
        every { authFlow.createKeyAccount("ana123") } returns AuthApiResult.Err("username-taken")
        viewModel.onUsernameChange("ana123")
        viewModel.onCreateAccount()
        advanceUntilIdle()
        val state = viewModel.uiState.value
        assertEquals(AuthStep.USERNAME_ENTRY, state.step)
        assertEquals(AuthUiError.UsernameTaken, state.error)
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
        advanceUntilIdle()
        every { authFlow.createKeyAccount("ana123") } returns AuthApiResult.Ok(AuthFlow.KeyIssued("ana123", "key-for-ana123"))
        every { authFlow.loginWithKey("key-for-ana123") } returns AuthApiResult.Err(PortableAuth.UNAVAILABLE)
        every { authFlow.currentSession() } returns null
        viewModel.onUsernameChange("ana123")
        viewModel.onCreateAccount()
        viewModel.onCreateAccount()
        advanceUntilIdle()
        assertEquals(AuthStep.KEY_ISSUED, viewModel.uiState.value.step)
        assertEquals("key-for-ana123", viewModel.uiState.value.issuedKey)
        viewModel.onLoginWithKey()
        advanceUntilIdle()
        assertEquals(AuthStep.KEY_ISSUED, viewModel.uiState.value.step)
        assertEquals("key-for-ana123", viewModel.uiState.value.issuedKey)
        verify(exactly = 1) { authFlow.createKeyAccount(any()) }
    }

    @Test
    fun resumeReflectsLogoutFromAnotherDestination() = runTest {
        every { authFlow.rehydrate() } returns AuthFlow.AuthState.Active(session())
        viewModel = AuthViewModel(authFlow, ownerErasure).also { it.ioDispatcher = dispatcher }
        advanceUntilIdle()
        every { authFlow.rehydrate() } returns AuthFlow.AuthState.LoggedOut
        viewModel.refreshAccount()
        advanceUntilIdle()
        assertEquals(AuthStep.USERNAME_ENTRY, viewModel.uiState.value.step)
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
