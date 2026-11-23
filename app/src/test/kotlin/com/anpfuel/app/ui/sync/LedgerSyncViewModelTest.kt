package com.anpfuel.app.ui.sync

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.ledger.*
import com.anpfuel.data.local.preferences.LedgerSyncFlagStore
import com.anpfuel.data.worker.LedgerSyncScheduler
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.repository.LedgerConsentStatus
import com.anpfuel.domain.repository.LedgerSyncDenied
import com.anpfuel.domain.repository.LedgerSyncScope
import io.mockk.*
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.*

@OptIn(ExperimentalCoroutinesApi::class)
class LedgerSyncViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val status = mockk<GetLedgerSyncStatusUseCase>()
    private val resolve = mockk<ResolveLedgerConflictUseCase>()
    private val preview = mockk<PreviewLedgerRestoreUseCase>()
    private val confirm = mockk<ConfirmLedgerRestoreUseCase>()
    private val flags = mockk<LedgerSyncFlagStore>(relaxed = true)
    private val scope = mockk<LedgerSyncScope>(relaxed = true)
    private val sessions = mockk<AuthSessionStore>()
    private val scheduler = mockk<LedgerSyncScheduler>(relaxed = true)
    private val consent = mockk<SetLedgerConsentUseCase>()
    private val session = PortableAuth.Session("f", "a", "t", "r", Long.MAX_VALUE, Long.MAX_VALUE)
    private val conflict = LedgerConflictRecord(LedgerDataset.REFUEL, "record")
    private val diff = RestoreDiff(emptyList(), emptyMap(), 0, 0, 0, false, "a", "o", 1)
    private fun summary(master: Boolean = true, dataset: Boolean = true, account: String? = "a") =
        LedgerSyncStatus(account, master, true, listOf(DatasetSyncStatus(LedgerDataset.REFUEL, 2, 1, null, null, dataset)), listOf(conflict))
    private fun model() = LedgerSyncViewModel(status, resolve, preview, confirm, flags, scope, sessions, scheduler, consent)

    @BeforeEach fun setup() {
        Dispatchers.setMain(dispatcher)
        every { sessions.load() } returns session
        every { flags.isEnabled() } returns true
        every { scope.isDeviceEnabled() } returns true
        every { scope.isDatasetEnabled(any()) } returns true
        coEvery { status.invoke() } returns summary()
        coEvery { preview.invoke() } returns diff
        coEvery { confirm.invoke(any()) } returns RestoreOutcome.NothingToApply
        coEvery { consent.invoke(any(), any()) } returns LedgerConsentStatus(1, LedgerDataset.entries)
    }
    @AfterEach fun teardown() { Dispatchers.resetMain() }

    @Test fun `master opt-in gates restore and disabling cancels without enqueue`() = runTest(dispatcher) {
        val vm = model(); vm.refresh(); advanceUntilIdle()
        every { flags.isEnabled() } returns false
        vm.previewRestore(); advanceUntilIdle()
        coVerify(exactly = 0) { preview.invoke() }
        vm.setMasterEnabled(false); advanceUntilIdle()
        verify { flags.setEnabled(false); scheduler.cancel() }
        verify(exactly = 0) { scheduler.enqueue(any()) }
    }

    @Test fun `dataset revocation refreshes status and blocks that conflict`() = runTest(dispatcher) {
        val vm = model(); vm.refresh(); advanceUntilIdle()
        every { scope.isDatasetEnabled("refuel") } returns false
        coEvery { status.invoke() } returns summary(dataset = false)
        vm.setDatasetEnabled(LedgerDataset.REFUEL, false); advanceUntilIdle()
        assertFalse(vm.uiState.value.status!!.datasets.single().enabled)
        vm.resolveConflict(conflict, LedgerConflictChoice.KEEP_MINE); advanceUntilIdle()
        coVerify(exactly = 0) { resolve.invoke(any(), any(), any()) }
        verify { scheduler.cancel() }
    }

    @Test fun `keep mine and take server are explicit while cancel preserves held conflict`() = runTest(dispatcher) {
        coEvery { resolve.invoke(any(), any(), LedgerConflictChoice.KEEP_MINE) } returns ResolveOutcome.AppliedKeepMine(2)
        coEvery { resolve.invoke(any(), any(), LedgerConflictChoice.TAKE_SERVER) } returns ResolveOutcome.AppliedTakeServer
        val vm = model(); vm.refresh(); advanceUntilIdle()
        vm.resolveConflict(conflict, LedgerConflictChoice.CANCEL); advanceUntilIdle()
        coVerify(exactly = 0) { resolve.invoke(any(), any(), any()) }
        assertEquals(1, vm.uiState.value.status!!.conflictRecords.size)
        vm.resolveConflict(conflict, LedgerConflictChoice.KEEP_MINE); advanceUntilIdle()
        vm.resolveConflict(conflict, LedgerConflictChoice.TAKE_SERVER); advanceUntilIdle()
        coVerify(exactly = 1) { resolve.invoke(LedgerDataset.REFUEL, "record", LedgerConflictChoice.KEEP_MINE) }
        coVerify(exactly = 1) { resolve.invoke(LedgerDataset.REFUEL, "record", LedgerConflictChoice.TAKE_SERVER) }
        verify(exactly = 0) { scheduler.enqueue(any()) }
    }

    @Test fun `preview requires confirmation and double confirm converges`() = runTest(dispatcher) {
        val vm = model(); vm.refresh(); advanceUntilIdle()
        vm.previewRestore(); advanceUntilIdle()
        assertEquals(diff, vm.uiState.value.staged)
        coVerify(exactly = 0) { confirm.invoke(any()) }
        vm.confirmRestore(); vm.confirmRestore(); advanceUntilIdle()
        coVerify(exactly = 1) { confirm.invoke(diff) }
        assertNull(vm.uiState.value.staged)
        assertEquals(SyncNotice.RESTORED, vm.uiState.value.notice)
    }

    @Test fun `stale confirmation drops staged preview and asks for another`() = runTest(dispatcher) {
        coEvery { confirm.invoke(any()) } returns RestoreOutcome.StaleSnapshot
        val vm = model(); vm.refresh(); advanceUntilIdle(); vm.previewRestore(); advanceUntilIdle()
        vm.confirmRestore(); advanceUntilIdle()
        assertEquals(SyncNotice.STALE, vm.uiState.value.notice)
        assertNull(vm.uiState.value.staged)
    }

    @Test fun `process restart loses staged authority and cancel never writes`() = runTest(dispatcher) {
        val first = model(); first.refresh(); advanceUntilIdle(); first.previewRestore(); advanceUntilIdle()
        val reopened = model(); reopened.refresh(); advanceUntilIdle()
        assertNull(reopened.uiState.value.staged)
        reopened.confirmRestore(); first.cancelRestore(); advanceUntilIdle()
        assertNull(first.uiState.value.staged)
        coVerify(exactly = 0) { confirm.invoke(any()) }
    }

    @Test fun `owner switch rejects old preview and clears private status on refresh`() = runTest(dispatcher) {
        val vm = model(); vm.refresh(); advanceUntilIdle(); vm.previewRestore(); advanceUntilIdle()
        every { sessions.load() } returns session.copy(accountId = "b")
        vm.confirmRestore(); advanceUntilIdle()
        coVerify(exactly = 0) { confirm.invoke(any()) }
        coEvery { status.invoke() } returns summary(account = "b")
        vm.refresh(); advanceUntilIdle()
        assertEquals("b", vm.uiState.value.status!!.accountId)
        assertNull(vm.uiState.value.staged)
    }

    @Test fun `late preview after owner switch cannot render old private rows`() = runTest(dispatcher) {
        val pending = CompletableDeferred<RestoreDiff>()
        coEvery { preview.invoke() } coAnswers { pending.await() }
        val vm = model(); vm.refresh(); advanceUntilIdle(); vm.previewRestore(); runCurrent()
        every { sessions.load() } returns session.copy(accountId = "b")
        pending.complete(diff); advanceUntilIdle()
        assertNull(vm.uiState.value.staged)
        assertNull(vm.uiState.value.status)
        assertEquals(SyncNotice.STALE, vm.uiState.value.notice)
    }

    @Test fun `offline empty and partial outcomes are visible and retryable`() = runTest(dispatcher) {
        val vm = model(); vm.refresh(); advanceUntilIdle()
        coEvery { preview.invoke() } throws java.io.IOException("offline")
        vm.previewRestore(); advanceUntilIdle()
        assertEquals(SyncNotice.OFFLINE, vm.uiState.value.notice)
        assertFalse(vm.uiState.value.busy)
        coEvery { preview.invoke() } returns diff
        vm.previewRestore(); advanceUntilIdle()
        assertTrue(vm.uiState.value.staged!!.toPull.isEmpty())
        coEvery { confirm.invoke(any()) } returns RestoreOutcome.Applied(listOf(RestoreItemResult(LedgerDataset.REFUEL, "r", RestoreItemOutcome.UNDECODABLE)))
        vm.confirmRestore(); advanceUntilIdle()
        assertEquals(SyncNotice.PARTIAL, vm.uiState.value.notice)
    }

    @Test fun `opting in records server consent for this device's datasets before participating`() = runTest(dispatcher) {
        every { flags.isEnabled() } returns false
        every { scope.isDatasetEnabled("expense") } returns false
        val vm = model(); vm.refresh(); advanceUntilIdle()
        vm.setMasterEnabled(true); advanceUntilIdle()
        coVerifyOrder {
            consent.invoke(listOf(LedgerDataset.REFUEL, LedgerDataset.CHARGE), true)
            flags.setEnabled(true)
        }
        assertNull(vm.uiState.value.notice)
    }

    @Test fun `a refused or offline opt-in keeps participation off`() = runTest(dispatcher) {
        every { flags.isEnabled() } returns false
        val vm = model(); vm.refresh(); advanceUntilIdle()
        coEvery { consent.invoke(any(), any()) } throws java.io.IOException("offline")
        vm.setMasterEnabled(true); advanceUntilIdle()
        assertEquals(SyncNotice.OFFLINE, vm.uiState.value.notice)
        coEvery { consent.invoke(any(), any()) } throws LedgerSyncDenied("ledger.denied")
        vm.setMasterEnabled(true); advanceUntilIdle()
        assertEquals(SyncNotice.ERROR, vm.uiState.value.notice)
        verify(exactly = 0) { flags.setEnabled(true) }
        assertFalse(vm.uiState.value.busy)
    }

    @Test fun `opting in without a session or without datasets sends nothing`() = runTest(dispatcher) {
        every { flags.isEnabled() } returns false
        every { scope.isDatasetEnabled(any()) } returns false
        val vm = model(); vm.refresh(); advanceUntilIdle()
        vm.setMasterEnabled(true); advanceUntilIdle()
        verify { flags.setEnabled(true) }
        every { sessions.load() } returns null
        vm.setMasterEnabled(true); advanceUntilIdle()
        verify(exactly = 1) { flags.setEnabled(true) }
        coVerify(exactly = 0) { consent.invoke(any(), any()) }
    }

    @Test fun `opting out stops this device first and then revokes server consent`() = runTest(dispatcher) {
        coEvery { consent.invoke(any(), false) } throws java.io.IOException("offline")
        val vm = model(); vm.refresh(); advanceUntilIdle()
        vm.setMasterEnabled(false); advanceUntilIdle()
        coVerifyOrder {
            flags.setEnabled(false)
            scheduler.cancel()
            consent.invoke(LedgerDataset.entries, false)
        }
        verify(exactly = 0) { flags.setEnabled(true) }
        assertEquals(SyncNotice.OFFLINE, vm.uiState.value.notice)
    }

    @Test fun `a dataset turned on while participating is consented before it uploads`() = runTest(dispatcher) {
        val vm = model(); vm.refresh(); advanceUntilIdle()
        vm.setDatasetEnabled(LedgerDataset.CHARGE, true); advanceUntilIdle()
        coVerifyOrder {
            consent.invoke(listOf(LedgerDataset.CHARGE), true)
            scope.setDatasetEnabled("charge", true)
        }
        coEvery { consent.invoke(any(), any()) } throws LedgerSyncDenied("ledger.denied")
        vm.setDatasetEnabled(LedgerDataset.EXPENSE, true); advanceUntilIdle()
        verify(exactly = 0) { scope.setDatasetEnabled("expense", true) }
        assertEquals(SyncNotice.ERROR, vm.uiState.value.notice)
    }

    @Test fun `dataset changes outside participation and dataset revocation stay local`() = runTest(dispatcher) {
        val vm = model(); vm.refresh(); advanceUntilIdle()
        vm.setDatasetEnabled(LedgerDataset.CHARGE, false); advanceUntilIdle()
        every { flags.isEnabled() } returns false
        vm.setDatasetEnabled(LedgerDataset.EXPENSE, true); advanceUntilIdle()
        verify { scope.setDatasetEnabled("charge", false); scope.setDatasetEnabled("expense", true) }
        coVerify(exactly = 0) { consent.invoke(any(), any()) }
    }

    @Test fun `an owner switch during opt-in leaves participation off`() = runTest(dispatcher) {
        every { flags.isEnabled() } returns false
        val pending = CompletableDeferred<LedgerConsentStatus>()
        coEvery { consent.invoke(any(), any()) } coAnswers { pending.await() }
        val vm = model(); vm.refresh(); advanceUntilIdle()
        vm.setMasterEnabled(true); runCurrent()
        every { sessions.load() } returns session.copy(accountId = "b")
        pending.complete(LedgerConsentStatus(1, LedgerDataset.entries)); advanceUntilIdle()
        verify(exactly = 0) { flags.setEnabled(true) }
        assertEquals(SyncNotice.STALE, vm.uiState.value.notice)
    }

    @Test fun `device revocation drops preview and stops existing work`() = runTest(dispatcher) {
        val vm = model(); vm.refresh(); advanceUntilIdle(); vm.previewRestore(); advanceUntilIdle()
        every { scope.isDeviceEnabled() } returns false
        vm.setDeviceEnabled(false); advanceUntilIdle()
        assertNull(vm.uiState.value.staged)
        vm.previewRestore(); advanceUntilIdle()
        coVerify(exactly = 1) { preview.invoke() }
        verify { scheduler.cancel() }
    }
}
