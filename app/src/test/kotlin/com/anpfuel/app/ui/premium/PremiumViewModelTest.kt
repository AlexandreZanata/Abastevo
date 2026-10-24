package com.anpfuel.app.ui.premium

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.premium.GetPremiumStatusUseCase
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.premium.PremiumState
import com.anpfuel.domain.premium.PremiumStatus
import com.anpfuel.domain.premium.PremiumStatusException
import com.anpfuel.domain.premium.PremiumStatusSnapshot
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class PremiumViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val useCase = mockk<GetPremiumStatusUseCase>()
    private val sessions = mockk<AuthSessionStore>()
    private val session = PortableAuth.Session("fam", "acct", "token", "refresh", Long.MAX_VALUE, Long.MAX_VALUE)
    private val active = PremiumStatus(
        policyVersion = 1, state = PremiumState.ACTIVE, reason = "verified",
        capabilities = setOf("cloud.sync"), validUntil = "2026-11-30T23:59:59Z",
        retainedDataAccess = true, paymentsConfigured = false,
        managementUrl = PremiumStatus.MANAGEMENT_URL,
    )

    @BeforeEach fun setup() { Dispatchers.setMain(dispatcher) }
    @AfterEach fun teardown() { Dispatchers.resetMain() }

    private fun model() = PremiumViewModel(useCase, sessions)

    @Test fun `signed out state makes no private call`() = runTest(dispatcher) {
        every { sessions.load() } returns null
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertTrue(vm.uiState.value.signedOut)
        coVerify(exactly = 0) { useCase.invoke() }
    }

    @Test fun `verified active status is rendered`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("acct", active)
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertEquals(PremiumState.ACTIVE, vm.uiState.value.status!!.state)
        assertFalse(vm.uiState.value.loading)
    }

    @Test fun `offline and denied status never retain a stale grant`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { useCase.invoke() } throws PremiumStatusException.Unavailable("down")
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertNull(vm.uiState.value.status)
        assertEquals(PremiumNotice.OFFLINE, vm.uiState.value.notice)
        coEvery { useCase.invoke() } throws PremiumStatusException.Denied("no")
        vm.refresh()
        advanceUntilIdle()
        assertEquals(PremiumNotice.DENIED, vm.uiState.value.notice)
    }

    @Test fun `account switch during request discards the old status`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("acct", active)
        val vm = model()
        vm.refresh()
        every { sessions.load() } returns session.copy(accountId = "other")
        advanceUntilIdle()
        assertTrue(vm.uiState.value.signedOut)
        assertNull(vm.uiState.value.status)
    }

    @Test fun `refresh for a new owner clears the previous status before the response`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("acct", active)
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertEquals(PremiumState.ACTIVE, vm.uiState.value.status!!.state)

        every { sessions.load() } returns session.copy(accountId = "other")
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot(
            "other",
            active.copy(state = PremiumState.PAUSED),
        )
        vm.refresh()
        assertTrue(vm.uiState.value.loading)
        assertNull(vm.uiState.value.status)

        advanceUntilIdle()
        assertFalse(vm.uiState.value.signedOut)
        assertEquals(PremiumState.PAUSED, vm.uiState.value.status!!.state)
    }

    @Test fun `logout clears the previous verified status on the next refresh`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("acct", active)
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertEquals(PremiumState.ACTIVE, vm.uiState.value.status!!.state)

        every { sessions.load() } returns null
        vm.refresh()
        advanceUntilIdle()
        assertTrue(vm.uiState.value.signedOut)
        assertNull(vm.uiState.value.status)
        coVerify(exactly = 1) { useCase.invoke() }
    }

    @Test fun `restore stays explicitly unavailable without a provider token`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("acct", active)
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        vm.restoreUnavailable()
        assertEquals(PremiumNotice.RESTORE_UNAVAILABLE, vm.uiState.value.notice)
        assertEquals(PremiumState.ACTIVE, vm.uiState.value.status!!.state)
        vm.clearNotice()
        assertNull(vm.uiState.value.notice)
    }

    @Test fun `store handoff failure is recoverable and keeps the verified status`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("acct", active)
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        vm.storeOpenFailed()
        assertEquals(PremiumNotice.STORE_FAILED, vm.uiState.value.notice)
        assertEquals(PremiumState.ACTIVE, vm.uiState.value.status!!.state)
        vm.clearNotice()
        assertNull(vm.uiState.value.notice)
    }
}
