package com.anpfuel.application.usecase.ledger

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.PremiumCommand
import com.anpfuel.domain.premium.PremiumOwnerScope
import com.anpfuel.domain.repository.LedgerSyncConflict
import com.anpfuel.domain.repository.LedgerSyncGateway
import com.anpfuel.domain.repository.LedgerSyncReceipt
import com.anpfuel.domain.repository.PremiumCommandStore
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import java.time.Instant
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class DispatchLedgerCommandUseCaseTest {

    private val commands = mockk<PremiumCommandStore>()
    private val gateway = mockk<LedgerSyncGateway>()
    private val sessions = mockk<AuthSessionStore>()

    private lateinit var useCase: DispatchLedgerCommandUseCase

    private val scope = PremiumOwnerScope("acct-1", "https://api.example.test")

    private fun command(id: String = "op-1") = PremiumCommand(
        operationId = id, scope = scope, dataset = LedgerDataset.REFUEL.wire,
        recordId = "rec-1", expectedRevision = 0, payloadJson = "{}",
    )

    private fun session() = com.anpfuel.domain.portable.PortableAuth.Session(
        familyId = "fam-1", accountId = "acct-1", accessToken = "at", refreshToken = "rt",
        accessExpiresAt = 0, absoluteExpiresAt = 0,
    )

    @BeforeEach
    fun setUp() {
        useCase = DispatchLedgerCommandUseCase(commands, gateway, sessions, "https://api.example.test", nowMillis = { 1000L })
        coEvery { commands.claim(any(), any(), any(), any()) } returns true
        coEvery { commands.acknowledge(any(), any()) } returns Unit
        coEvery { commands.failTransient(any(), any(), any(), any()) } returns Unit
        coEvery { commands.markConflict(any(), any(), any()) } returns Unit
    }

    @Test fun `no session means no io and pending kept`() = runTest {
        every { sessions.load() } returns null
        coEvery { commands.listPending(any(), any()) } returns listOf(command())
        assertEquals(LedgerDispatchOutcome.REFUSED, useCase.invoke(null))
        coVerify(exactly = 0) { gateway.submit(any(), any(), any(), any(), any()) }
    }

    @Test fun `accepted and replayed commands are acknowledged once`() = runTest {
        every { sessions.load() } returns session()
        coEvery { commands.listPending(any(), any()) } returns listOf(command())
        coEvery { commands.claim(any(), any(), any(), any()) } returns true
        coEvery { gateway.submit(any(), any(), any(), any(), any()) } returns LedgerSyncReceipt("op-1", 1L, true)
        assertEquals(LedgerDispatchOutcome.DONE, useCase.invoke(null))
        coVerify(exactly = 1) { commands.acknowledge(any(), "op-1") }
        coEvery { commands.listPending(any(), any()) } returns listOf(command())
        coEvery { gateway.submit(any(), any(), any(), any(), any()) } returns LedgerSyncReceipt("op-1", 1L, true)
        assertEquals(LedgerDispatchOutcome.DONE, useCase.invoke(null))
        coVerify(exactly = 2) { commands.acknowledge(any(), "op-1") }
    }

    @Test fun `lost reply retries with the same operation id`() = runTest {
        every { sessions.load() } returns session()
        val slot = io.mockk.slot<com.anpfuel.domain.premium.PremiumCommand>()
        coEvery { commands.listPending(any(), any()) } returns listOf(command())
        coEvery { commands.claim(any(), any(), any(), any()) } returns true
        coEvery { gateway.submit(any(), capture(slot), any(), any(), any()) } throws java.io.IOException("reply lost")
        assertEquals(LedgerDispatchOutcome.RETRY, useCase.invoke(null))
        assertEquals("op-1", slot.captured.operationId)
        coVerify(exactly = 1) { commands.failTransient(any(), "op-1", any(), any()) }
    }

    @Test fun `stale revision parks as conflict without deleting`() = runTest {
        every { sessions.load() } returns session()
        coEvery { commands.listPending(any(), any()) } returns listOf(command())
        coEvery { commands.claim(any(), any(), any(), any()) } returns true
        coEvery { gateway.submit(any(), any(), any(), any(), any()) } throws LedgerSyncConflict("stale revision")
        assertEquals(LedgerDispatchOutcome.REFUSED, useCase.invoke(null))
        coVerify(exactly = 1) { commands.markConflict(any(), "op-1", any()) }
        coVerify(exactly = 0) { commands.acknowledge(any(), any()) }
    }

    @Test fun `duplicate worker loses the claim race`() = runTest {
        every { sessions.load() } returns session()
        coEvery { commands.listPending(any(), any()) } returns listOf(command())
        coEvery { commands.claim(any(), any(), any(), any()) } returns false
        assertEquals(LedgerDispatchOutcome.RETRY, useCase.invoke(null))
        coVerify(exactly = 0) { gateway.submit(any(), any(), any(), any(), any()) }
    }

    @Test fun `opted out queue stays pending without io`() = runTest {
        val gated = DispatchLedgerCommandUseCase(commands, gateway, sessions, "https://api.example.test", nowMillis = { 1000L }, enabled = { false })
        coEvery { commands.listPending(any(), any()) } returns listOf(command())
        assertEquals(LedgerDispatchOutcome.DONE, gated.invoke(null))
        coVerify(exactly = 0) { gateway.submit(any(), any(), any(), any(), any()) }
    }
}
