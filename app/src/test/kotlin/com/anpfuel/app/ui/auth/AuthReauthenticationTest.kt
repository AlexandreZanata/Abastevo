package com.anpfuel.app.ui.auth

import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.data.repository.OwnerLocalErasure
import com.anpfuel.domain.portable.PortableAuth
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
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class AuthReauthenticationTest {
    private val dispatcher = StandardTestDispatcher()
    @BeforeEach fun setup() { Dispatchers.setMain(dispatcher) }
    @AfterEach fun cleanup() { Dispatchers.resetMain() }

    @Test fun `explicit fresh key entry supersedes pending account refresh without logout`() = runTest(dispatcher) {
        val auth = mockk<AuthFlow>(relaxed = true)
        val erasure = mockk<OwnerLocalErasure>(relaxed = true)
        val session = PortableAuth.Session("family", "account", "access", "refresh", Long.MAX_VALUE, Long.MAX_VALUE)
        every { auth.rehydrate() } returns AuthFlow.AuthState.Active(session)
        val vm = AuthViewModel(auth, erasure).also { it.ioDispatcher = dispatcher }
        // The initializer's queued refresh must not replace this deliberate form.
        vm.beginReauthentication(); vm.onKeyChange("synthetic-entered-key")
        advanceUntilIdle()
        assertEquals(AuthStep.REAUTHENTICATION, vm.uiState.value.step)
        assertEquals("synthetic-entered-key", vm.uiState.value.accountKey)
        verify(exactly = 0) { auth.logout() }
        assertNull(vm.uiState.value.keyBackup)
    }
    @Test
    fun `company sign in still accepts a key when there is no personal session`() = runTest(dispatcher) {
        val auth = mockk<AuthFlow>()
        val erasure = mockk<OwnerLocalErasure>()
        val session = PortableAuth.Session("family", "account", "access", "refresh", Long.MAX_VALUE, Long.MAX_VALUE)
        every { auth.rehydrate() } returns AuthFlow.AuthState.LoggedOut
        every { auth.profile() } returns AuthApiResult.Err(PortableAuth.Verdict.SESSION_EXPIRED)
        every { auth.currentKey() } returns null
        every { auth.loginWithKey("synthetic-entered-key") } returns AuthApiResult.Ok(AuthFlow.Login(session, false))
        val vm = AuthViewModel(auth, erasure).also { it.ioDispatcher = dispatcher }
        vm.beginReauthentication()
        advanceUntilIdle()
        vm.onKeyChange("synthetic-entered-key")
        vm.onLoginWithKey()
        advanceUntilIdle()
        assertEquals(AuthStep.AUTHENTICATED, vm.uiState.value.step)
        verify(exactly = 1) { auth.loginWithKey("synthetic-entered-key") }
    }
}
