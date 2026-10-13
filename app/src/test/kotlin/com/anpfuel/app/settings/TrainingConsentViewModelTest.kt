package com.anpfuel.app.settings

import com.anpfuel.application.usecase.privacy.TrainingConsentCaller
import com.anpfuel.application.usecase.privacy.TrainingConsentOutcome
import com.anpfuel.application.usecase.privacy.TrainingConsentUseCase
import com.anpfuel.domain.repository.TrainingConsentRejectKind
import com.anpfuel.domain.repository.TrainingConsentStatus
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

/**
 * CS11B: guests see the login explanation with no network IO; the
 * switch loads the owner status once per foreground; toggling ON
 * grants then reloads the exact server state; toggling OFF revokes
 * then reloads; a second tap mid-flight is ignored; offline and
 * pending states stay explicit with retry; rejections stay explicit
 * and never claim consent.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class TrainingConsentViewModelTest {

    private val dispatcher = StandardTestDispatcher()

    private val useCase = mockk<TrainingConsentUseCase>()
    private val caller = TrainingConsentCaller("acct-1")

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    private fun grantedStatus() = TrainingConsentStatus(
        granted = true,
        state = "GRANTED",
        termsVersion = "training-terms-v1",
        termsDigest = "digest-v1",
        revision = 1,
        needsRegrant = false,
    )

    private fun offStatus() = TrainingConsentStatus(
        granted = false,
        state = null,
        termsVersion = null,
        termsDigest = null,
        revision = 0,
        needsRegrant = false,
    )

    private fun preparedVm(accountId: String = "acct-1"): TrainingConsentViewModel {
        coEvery { useCase.syncPending(any()) } returns
            TrainingConsentOutcome.Status(grantedStatus(), false)
        val vm = TrainingConsentViewModel(useCase).also { it.ioDispatcher = dispatcher }
        vm.prepare(accountId = accountId)
        return vm
    }

    @Test
    fun `guest never calls use case`() = runTest(dispatcher) {
        val vm = TrainingConsentViewModel(useCase).also { it.ioDispatcher = dispatcher }
        vm.prepare(accountId = "")
        advanceUntilIdle()
        assertTrue(vm.state.value is TrainingConsentUiState.Guest)
        coVerify(exactly = 0) { useCase.syncPending(any()) }
    }

    @Test
    fun `foreground loads content once`() = runTest(dispatcher) {
        val vm = preparedVm()
        advanceUntilIdle()
        val state = vm.state.value
        assertTrue(state is TrainingConsentUiState.Content)
        assertEquals(true, (state as TrainingConsentUiState.Content).granted)
        coVerify(exactly = 1) { useCase.syncPending(caller) }
    }

    @Test
    fun `toggle on grants then reloads`() = runTest(dispatcher) {
        coEvery { useCase.syncPending(any()) } returns
            TrainingConsentOutcome.Status(offStatus(), false)
        coEvery { useCase.grant(any(), any(), any()) } returns
            TrainingConsentOutcome.Granted(
                com.anpfuel.domain.repository.TrainingConsentReceipt("GRANTED", 1, "training-terms-v1", false),
            )
        val vm = TrainingConsentViewModel(useCase).also { it.ioDispatcher = dispatcher }
        vm.prepare(accountId = "acct-1")
        advanceUntilIdle()
        coEvery { useCase.syncPending(any()) } returns
            TrainingConsentOutcome.Status(grantedStatus(), false)
        vm.setEnabled(true, "training-terms-v1", "digest-v1")
        advanceUntilIdle()
        val state = vm.state.value
        assertTrue(state is TrainingConsentUiState.Content)
        assertEquals(true, (state as TrainingConsentUiState.Content).granted)
        coVerify { useCase.grant(caller, "training-terms-v1", "digest-v1") }
    }

    @Test
    fun `toggle off revokes then reloads`() = runTest(dispatcher) {
        val vm = preparedVm()
        advanceUntilIdle()
        coEvery { useCase.revoke(any()) } returns
            TrainingConsentOutcome.Revoked(
                com.anpfuel.domain.repository.TrainingConsentReceipt("REVOKED", 2, "training-terms-v1", false, true),
            )
        coEvery { useCase.syncPending(any()) } returns
            TrainingConsentOutcome.Status(offStatus(), false)
        vm.setEnabled(false, "training-terms-v1", "digest-v1")
        advanceUntilIdle()
        val state = vm.state.value
        assertTrue(state is TrainingConsentUiState.Content)
        assertEquals(false, (state as TrainingConsentUiState.Content).granted)
        coVerify { useCase.revoke(caller) }
    }

    @Test
    fun `second tap mid flight ignored`() = runTest(dispatcher) {
        val vm = preparedVm()
        advanceUntilIdle()
        coEvery { useCase.grant(any(), any(), any()) } returns
            TrainingConsentOutcome.Granted(
                com.anpfuel.domain.repository.TrainingConsentReceipt("GRANTED", 1, "training-terms-v1", false),
            )
        vm.setEnabled(true, "training-terms-v1", "digest-v1")
        vm.setEnabled(true, "training-terms-v1", "digest-v1")
        advanceUntilIdle()
        coVerify(exactly = 1) { useCase.grant(any(), any(), any()) }
    }

    @Test
    fun `offline stays explicit with retry`() = runTest(dispatcher) {
        coEvery { useCase.syncPending(any()) } returns TrainingConsentOutcome.Offline
        val vm = TrainingConsentViewModel(useCase).also { it.ioDispatcher = dispatcher }
        vm.prepare(accountId = "acct-1")
        advanceUntilIdle()
        assertTrue(vm.state.value is TrainingConsentUiState.Offline)
        coEvery { useCase.syncPending(any()) } returns
            TrainingConsentOutcome.Status(grantedStatus(), false)
        vm.retry()
        advanceUntilIdle()
        assertTrue(vm.state.value is TrainingConsentUiState.Content)
    }

    @Test
    fun `pending revocation stays explicit`() = runTest(dispatcher) {
        coEvery { useCase.syncPending(any()) } returns TrainingConsentOutcome.RevocationPending
        val vm = TrainingConsentViewModel(useCase).also { it.ioDispatcher = dispatcher }
        vm.prepare(accountId = "acct-1")
        advanceUntilIdle()
        assertTrue(vm.state.value is TrainingConsentUiState.Pending)
    }

    @Test
    fun `rejections stay explicit`() = runTest(dispatcher) {
        coEvery { useCase.syncPending(any()) } returns
            TrainingConsentOutcome.Rejected(TrainingConsentRejectKind.STALE_REVISION, "stale")
        val vm = TrainingConsentViewModel(useCase).also { it.ioDispatcher = dispatcher }
        vm.prepare(accountId = "acct-1")
        advanceUntilIdle()
        val state = vm.state.value
        assertTrue(state is TrainingConsentUiState.Rejected)
        assertEquals(
            TrainingConsentRejectKind.STALE_REVISION,
            (state as TrainingConsentUiState.Rejected).kind,
        )
    }

    @Test
    fun `late refresh cannot disclose previous account state`() = runTest(dispatcher) {
        val old = CompletableDeferred<TrainingConsentOutcome>()
        coEvery { useCase.syncPending(TrainingConsentCaller("acct-A")) } coAnswers { old.await() }
        coEvery { useCase.syncPending(TrainingConsentCaller("acct-B")) } returns TrainingConsentOutcome.Status(offStatus(), false)
        val vm = TrainingConsentViewModel(useCase).also { it.ioDispatcher = dispatcher }
        vm.prepare("acct-A")
        advanceUntilIdle()
        vm.prepare("acct-B")
        advanceUntilIdle()
        old.complete(TrainingConsentOutcome.Status(grantedStatus(), false))
        advanceUntilIdle()
        assertEquals(false, (vm.state.value as TrainingConsentUiState.Content).granted)
    }

    @Test
    fun `logout discards in flight result and clears retry authority`() = runTest(dispatcher) {
        val old = CompletableDeferred<TrainingConsentOutcome>()
        coEvery { useCase.syncPending(any()) } coAnswers { old.await() }
        val vm = TrainingConsentViewModel(useCase).also { it.ioDispatcher = dispatcher }
        vm.prepare("acct-A")
        advanceUntilIdle()
        vm.prepare("")
        old.complete(TrainingConsentOutcome.Status(grantedStatus(), false))
        advanceUntilIdle()
        assertEquals(TrainingConsentUiState.Guest, vm.state.value)
        vm.retry()
        vm.setEnabled(false, "v1", "digest")
        advanceUntilIdle()
        coVerify(exactly = 1) { useCase.syncPending(any()) }
        coVerify(exactly = 0) { useCase.revoke(any()) }
    }

    @Test
    fun `old update cannot reload or release new owner update guard`() = runTest(dispatcher) {
        val old = CompletableDeferred<TrainingConsentOutcome>()
        val next = CompletableDeferred<TrainingConsentOutcome>()
        coEvery { useCase.syncPending(any()) } returns TrainingConsentOutcome.Status(offStatus(), false)
        coEvery { useCase.revoke(TrainingConsentCaller("acct-A")) } coAnswers { old.await() }
        coEvery { useCase.revoke(TrainingConsentCaller("acct-B")) } coAnswers { next.await() }
        val vm = TrainingConsentViewModel(useCase).also { it.ioDispatcher = dispatcher }
        vm.prepare("acct-A")
        advanceUntilIdle()
        vm.setEnabled(false, "v1", "digest")
        advanceUntilIdle()
        vm.prepare("acct-B")
        advanceUntilIdle()
        assertTrue(vm.state.value is TrainingConsentUiState.Content)
        vm.setEnabled(false, "v1", "digest")
        advanceUntilIdle()
        old.complete(TrainingConsentOutcome.Revoked(
            com.anpfuel.domain.repository.TrainingConsentReceipt("REVOKED", 2, "v1", false, true),
        ))
        advanceUntilIdle()
        assertEquals(TrainingConsentUiState.Updating, vm.state.value)
        vm.setEnabled(false, "v1", "digest")
        advanceUntilIdle()
        coVerify(exactly = 1) { useCase.revoke(TrainingConsentCaller("acct-B")) }
        coVerify(exactly = 1) { useCase.syncPending(TrainingConsentCaller("acct-A")) }
        next.complete(TrainingConsentOutcome.RevocationPending)
        advanceUntilIdle()
        assertEquals(TrainingConsentUiState.Pending, vm.state.value)
    }

    @Test
    fun `older same owner refresh cannot overwrite newer refresh`() = runTest(dispatcher) {
        val old = CompletableDeferred<TrainingConsentOutcome>()
        var calls = 0
        coEvery { useCase.syncPending(any()) } coAnswers {
            if (calls++ == 0) old.await() else TrainingConsentOutcome.Status(offStatus(), false)
        }
        val vm = TrainingConsentViewModel(useCase).also { it.ioDispatcher = dispatcher }
        vm.prepare("acct-A")
        advanceUntilIdle()
        vm.refresh()
        advanceUntilIdle()
        old.complete(TrainingConsentOutcome.Status(grantedStatus(), false))
        advanceUntilIdle()
        assertEquals(false, (vm.state.value as TrainingConsentUiState.Content).granted)
    }

    @Test fun `OFF can overtake an in flight ON and its late result cannot repaint`() = runTest(dispatcher) {
        val grant=CompletableDeferred<TrainingConsentOutcome>()
        coEvery {useCase.syncPending(any())} returns TrainingConsentOutcome.Status(offStatus(),false)
        coEvery {useCase.grant(any(),any(),any())} coAnswers {grant.await()}
        coEvery {useCase.revoke(any())} returns TrainingConsentOutcome.RevocationPending
        val vm=TrainingConsentViewModel(useCase).also {it.ioDispatcher=dispatcher}
        vm.prepare("acct-1");advanceUntilIdle()
        vm.setEnabled(true,"training-terms-v1","digest-v1");advanceUntilIdle()
        vm.setEnabled(false,"","");advanceUntilIdle()
        assertEquals(TrainingConsentUiState.Pending,vm.state.value)
        coVerify(exactly=1) {useCase.revoke(caller)}
        grant.complete(TrainingConsentOutcome.Granted(com.anpfuel.domain.repository.TrainingConsentReceipt("GRANTED",1,"training-terms-v1",false)))
        advanceUntilIdle();assertEquals(TrainingConsentUiState.Pending,vm.state.value)
    }

    @Test
    fun `repeat foreground entry within TTL reuses snapshot without polling`() = runTest(dispatcher) {
        coEvery { useCase.syncPending(any()) } returns
            TrainingConsentOutcome.Status(grantedStatus(), false)
        var now = 1_000_000L
        val vm = TrainingConsentViewModel(useCase).also {
            it.ioDispatcher = dispatcher
            it.clock = { now }
        }
        vm.prepare("acct-1")
        advanceUntilIdle()
        assertTrue(vm.state.value is TrainingConsentUiState.Content)
        vm.prepare("acct-1")
        advanceUntilIdle()
        assertTrue(vm.state.value is TrainingConsentUiState.Content)
        coVerify(exactly = 1) { useCase.syncPending(any()) }
    }

    @Test
    fun `foreground entry after TTL polls again`() = runTest(dispatcher) {
        coEvery { useCase.syncPending(any()) } returns
            TrainingConsentOutcome.Status(grantedStatus(), false)
        var now = 1_000_000L
        val vm = TrainingConsentViewModel(useCase).also {
            it.ioDispatcher = dispatcher
            it.clock = { now }
        }
        vm.prepare("acct-1")
        advanceUntilIdle()
        now += TrainingConsentViewModel.STATUS_CACHE_TTL_MILLIS
        vm.prepare("acct-1")
        advanceUntilIdle()
        assertTrue(vm.state.value is TrainingConsentUiState.Content)
        coVerify(exactly = 2) { useCase.syncPending(any()) }
    }

    @Test
    fun `account change always polls despite fresh snapshot`() = runTest(dispatcher) {
        coEvery { useCase.syncPending(any()) } returns
            TrainingConsentOutcome.Status(grantedStatus(), false)
        var now = 1_000_000L
        val vm = TrainingConsentViewModel(useCase).also {
            it.ioDispatcher = dispatcher
            it.clock = { now }
        }
        vm.prepare("acct-A")
        advanceUntilIdle()
        vm.prepare("acct-B")
        advanceUntilIdle()
        assertTrue(vm.state.value is TrainingConsentUiState.Content)
        coVerify(exactly = 2) { useCase.syncPending(any()) }
    }

}
