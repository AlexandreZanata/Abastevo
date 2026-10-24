package com.anpfuel.application.usecase.ledger

import com.anpfuel.application.port.LedgerSyncFlagProvider
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.premium.PremiumCommand
import com.anpfuel.domain.premium.PremiumOwnerScope
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.repository.LedgerAck as SyncAck
import com.anpfuel.domain.repository.LedgerSyncAckStore
import com.anpfuel.domain.repository.LedgerSyncScope
import com.anpfuel.domain.repository.PremiumCommandStore
import io.mockk.coEvery
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class GetLedgerSyncStatusUseCaseTest {

    private val commands = mockk<PremiumCommandStore>()
    private val acks = mockk<LedgerSyncAckStore>()
    private val sessions = mockk<AuthSessionStore>()
    private val flags = mockk<LedgerSyncFlagProvider>()
    private val scope = mockk<LedgerSyncScope>()

    private lateinit var useCase: GetLedgerSyncStatusUseCase

    private fun command(id: String, dataset: String) = PremiumCommand(
        operationId = id, scope = PremiumOwnerScope("acct-1", "https://api.example.test"),
        dataset = dataset, recordId = "r-$id", expectedRevision = 0, payloadJson = "{}",
    )

    @BeforeEach
    fun setUp() {
        every { sessions.load() } returns com.anpfuel.domain.portable.PortableAuth.Session(
            familyId = "fam-1", accountId = "acct-1", accessToken = "t", refreshToken = "r",
            accessExpiresAt = Long.MAX_VALUE, absoluteExpiresAt = Long.MAX_VALUE,
        )
        every { flags.isEnabled() } returns true
        every { scope.isDeviceEnabled() } returns true
        every { scope.isDatasetEnabled(any()) } returns true
        coEvery { acks.lastAck(any(), any(), any()) } returns null
        coEvery { commands.countPending(any(), any(), any()) } returns 0
        coEvery { commands.listConflicts(any(), any()) } returns emptyList()
        useCase = GetLedgerSyncStatusUseCase(commands, acks, sessions, "https://api.example.test", flags, scope)
    }

    @Test fun `signed out reports flags only`() = runTest {
        every { sessions.load() } returns null
        val status = useCase.invoke()
        assertEquals(null, status.accountId)
        assertEquals(0, status.datasets.size)
        coVerifyCount(0)
    }

    @Test fun `pending conflicts and acks report per dataset`() = runTest {
        coEvery { commands.countPending(any(), any(), "refuel") } returns 3
        coEvery { commands.countPending(any(), any(), "expense") } returns 1
        coEvery { commands.listConflicts(any(), any()) } returns listOf(
            command("op-1", LedgerDataset.REFUEL.wire),
            command("op-2", LedgerDataset.CHARGE.wire),
        )
        coEvery { acks.lastAck("acct-1", "https://api.example.test", LedgerDataset.REFUEL.wire) } returns
            SyncAck(LedgerDataset.REFUEL.wire, 7, 100L)
        val status = useCase.invoke()
        assertEquals("acct-1", status.accountId)
        val refuel = status.datasets.single { it.dataset == LedgerDataset.REFUEL }
        assertEquals(3, refuel.pending)
        assertEquals(1, refuel.conflicts)
        assertEquals(7L, refuel.lastAckRevision)
        val expense = status.datasets.single { it.dataset == LedgerDataset.EXPENSE }
        assertEquals(1, expense.pending)
        assertEquals(0, expense.conflicts)
        assertNull(expense.lastAckRevision)
    }

    private fun coVerifyCount(count: Int) {
        io.mockk.coVerify(exactly = count) { commands.countPending(any(), any(), any()) }
    }
}
