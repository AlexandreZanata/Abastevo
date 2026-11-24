package com.anpfuel.app.ui.auth

import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.GoogleLogin
import com.anpfuel.application.portable.GoogleSignInGateway
import com.anpfuel.application.portable.GoogleSignInResult
import com.anpfuel.data.repository.OwnerLocalErasure
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.portable.PortableAuth.CodePurpose
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class EmailAndGoogleAuthViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val auth = mockk<AuthFlow>()
    private val erasure = mockk<OwnerLocalErasure>(relaxed = true)
    private val email = "test@example.invalid"
    private val session = PortableAuth.Session("family", "account", "access", "refresh", 2000, 3000)
    private val profile = PortableAuth.Profile(
        "account", "Public Person", "Public Person", true, "t***@example.invalid", true, setOf("google"),
    )
    private val backup = AuthFlow.KeyBackup("Public Person", "synthetic-key")

    @BeforeEach
    fun setup() {
        Dispatchers.setMain(dispatcher)
        every { auth.rehydrate() } returns AuthFlow.AuthState.LoggedOut
        every { auth.currentKey() } returns null
        every { auth.profile() } returns AuthApiResult.Ok(profile)
    }

    @AfterEach
    fun teardown() = Dispatchers.resetMain()

    private fun viewModel() = AuthViewModel(auth, erasure).also { it.ioDispatcher = dispatcher }

    @Test
    fun signupAndRecoveryBothRetainTheIssuedKeyUntilAcknowledged() = runTest(dispatcher) {
        every { auth.requestCode(email, any(), "en") } returns AuthApiResult.Ok(Unit)
        every { auth.signUp("Public Person", email, "012345") } returns AuthApiResult.Ok(AuthFlow.NewKey(backup, true))
        every { auth.recover(email, "012345") } returns AuthApiResult.Ok(AuthFlow.NewKey(backup, true))
        every { auth.currentSession() } returns session
        for (purpose in listOf(CodePurpose.SIGNUP, CodePurpose.RECOVERY)) {
            val vm = viewModel()
            runCurrent()
            assertEquals(AuthStep.ENTRY, vm.uiState.value.step)
            if (purpose == CodePurpose.SIGNUP) {
                vm.beginSignUp()
                vm.onNameChange("Public Person")
                assertEquals(AuthStep.SIGNUP_FORM, vm.uiState.value.step)
            } else {
                vm.beginRecovery()
                assertEquals(AuthStep.RECOVERY_FORM, vm.uiState.value.step)
            }
            vm.onEmailChange(email)
            vm.onRequestCode("en")
            runCurrent()
            assertEquals(AuthStep.CODE_SENT, vm.uiState.value.step)
            assertEquals(60, vm.uiState.value.resendSeconds)
            vm.onCodeChange("012345")
            vm.onConsumeCode()
            runCurrent()
            assertEquals(AuthStep.KEY_ISSUED, vm.uiState.value.step)
            assertEquals(purpose, vm.uiState.value.purpose)
            assertEquals(backup.accountKey, vm.uiState.value.issuedKey)
            vm.onLoginWithKey()
            runCurrent()
            assertEquals(AuthStep.AUTHENTICATED, vm.uiState.value.step)
            assertEquals("", vm.uiState.value.issuedKey)
        }
    }

    @Test
    fun googleConfirmationUsesLocalDisplayNameAndSavesExplicitEdits() = runTest(dispatcher) {
        every { auth.loginWithGoogle("synthetic-id-token", any()) } returns AuthApiResult.Ok(GoogleLogin(session, true, false))
        every { auth.updateName("Public Person") } returns AuthApiResult.Ok(profile)
        val vm = viewModel()
        runCurrent()
        vm.onGoogle(GoogleSignInGateway { GoogleSignInResult.Selected("synthetic-id-token", "Suggested Name") })
        runCurrent()
        assertEquals(AuthStep.CONFIRM_NAME, vm.uiState.value.step)
        assertEquals("Suggested Name", vm.uiState.value.name)
        verify(exactly = 0) { auth.updateName(any()) }
        vm.onNameChange("Public Person")
        vm.onSaveName()
        runCurrent()
        assertEquals(AuthStep.AUTHENTICATED, vm.uiState.value.step)
        assertEquals(profile, vm.uiState.value.profile)
    }

    @Test
    fun invalidPublicNameIsRefusedBeforeAnEmailCodeIsRequested() = runTest(dispatcher) {
        val vm = viewModel()
        runCurrent()
        vm.beginSignUp()
        vm.onEmailChange(email)
        for (name in listOf("Suporte Abastevo", "João🟢", "A")) {
            vm.onNameChange(name)
            vm.onRequestCode("en")
            runCurrent()
            assertEquals(AuthStep.SIGNUP_FORM, vm.uiState.value.step)
            assertEquals(AuthUiError.InvalidName, vm.uiState.value.error)
        }
        verify(exactly = 0) { auth.requestCode(any(), any(), any()) }
    }

    @Test
    fun invalidPublicNameEditStaysOnTheFormWithoutCallingTheApi() = runTest(dispatcher) {
        every { auth.rehydrate() } returns AuthFlow.AuthState.Active(session)
        val vm = viewModel()
        runCurrent()
        vm.beginPublicNameEdit()
        vm.onNameChange("Admin")
        vm.onSaveName()
        runCurrent()
        assertEquals(AuthStep.EDIT_NAME, vm.uiState.value.step)
        assertEquals(AuthUiError.InvalidName, vm.uiState.value.error)
        verify(exactly = 0) { auth.updateName(any()) }
    }

    @Test
    fun googleCancellationIsSilentAndEmailConflictReturnsToEntry() = runTest(dispatcher) {
        every { auth.loginWithGoogle("synthetic-id-token", any()) } returns AuthApiResult.Err("provider-email-in-use")
        val vm = viewModel()
        runCurrent()
        vm.onGoogle(GoogleSignInGateway { GoogleSignInResult.Cancelled })
        runCurrent()
        assertEquals(AuthStep.ENTRY, vm.uiState.value.step)
        assertNull(vm.uiState.value.error)
        verify(exactly = 0) { auth.loginWithGoogle(any(), any()) }

        vm.onGoogle(GoogleSignInGateway { GoogleSignInResult.Selected("synthetic-id-token", null) })
        runCurrent()
        assertEquals(AuthStep.ENTRY, vm.uiState.value.step)
        assertEquals(AuthUiError.ProviderEmailInUse, vm.uiState.value.error)
    }

    @Test
    fun leavingNameConfirmationKeepsTheSessionAndPromptsOnReopening() = runTest(dispatcher) {
        every { auth.rehydrate() } returns AuthFlow.AuthState.Active(session)
        every { auth.profile() } returns AuthApiResult.Ok(profile.copy(name = null, profileComplete = false))
        val vm = viewModel()
        runCurrent()
        vm.resumePublicNameConfirmation()
        assertEquals(AuthStep.CONFIRM_NAME, vm.uiState.value.step)
        vm.onCancelProfileEdit()
        assertEquals(AuthStep.AUTHENTICATED, vm.uiState.value.step)
        verify(exactly = 0) { auth.logout() }
        vm.resumePublicNameConfirmation()
        assertEquals(AuthStep.CONFIRM_NAME, vm.uiState.value.step)
    }

    @Test
    fun staleRecoveryEmailMutationPromptsForNewProofAndPreservesTheCode() = runTest(dispatcher) {
        every { auth.rehydrate() } returns AuthFlow.AuthState.Active(session)
        every { auth.loginWithKey("synthetic-key", "account") } returns AuthApiResult.Ok(AuthFlow.Login(session, false))
        every { auth.requestCode(email, CodePurpose.ATTACH, "en") } returns AuthApiResult.Ok(Unit)
        every { auth.setRecoveryEmail(email, "012345") } returns AuthApiResult.Err("reauthentication-required")
        val vm = viewModel()
        runCurrent()
        vm.beginRecoveryEmailChange()
        assertEquals(AuthStep.REAUTHENTICATION, vm.uiState.value.step)
        vm.onKeyChange("synthetic-key")
        vm.onLoginWithKey()
        runCurrent()
        assertEquals(AuthStep.ATTACH_EMAIL, vm.uiState.value.step)
        vm.onEmailChange(email)
        vm.onRequestCode("en")
        runCurrent()
        vm.onCodeChange("012345")
        vm.onConsumeCode()
        runCurrent()
        assertEquals(AuthStep.REAUTHENTICATION, vm.uiState.value.step)
        assertEquals(AuthUiError.ReauthenticationRequired, vm.uiState.value.error)
        vm.onKeyChange("synthetic-key")
        vm.onLoginWithKey()
        runCurrent()
        assertEquals(AuthStep.CODE_SENT, vm.uiState.value.step)
        assertEquals("012345", vm.uiState.value.code)
    }
}
