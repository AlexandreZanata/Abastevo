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
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
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
    private val syncScope = mockk<com.anpfuel.domain.repository.LedgerSyncScope>()
    private val acks = mockk<com.anpfuel.domain.repository.LedgerSyncAckStore>(relaxed = true)

    private fun gated() = DispatchLedgerCommandUseCase(commands, gateway, sessions, scope.origin,
        nowMillis = { 1000L }, syncScope = syncScope, acks = acks)

    @Test fun `revoked head is skipped for allowed dataset`() = runTest {
        every { sessions.load() } returns session()
        every { syncScope.isDeviceEnabled() } returns true
        every { syncScope.isDatasetEnabled("refuel") } returns false
        every { syncScope.isDatasetEnabled("expense") } returns true
        val allowed = command("op-2").copy(dataset = "expense")
        coEvery { commands.listPending(any(), 1) } returns listOf(command())
        coEvery { commands.listPending(any(), 50) } returns listOf(command(), allowed)
        coEvery { gateway.submit(any(), any(), any(), any(), any()) } returns LedgerSyncReceipt("op-2", 4, false)
        assertEquals(LedgerDispatchOutcome.DONE, gated().invoke(null))
        coVerify(exactly = 1) { gateway.submit(any(), allowed, any(), any(), any()) }
    }

    @Test fun `disabled device performs no session or outbox io`() = runTest {
        every { syncScope.isDeviceEnabled() } returns false
        assertEquals(LedgerDispatchOutcome.DONE, gated().invoke(null))
        io.mockk.verify(exactly = 0) { sessions.load() }
        coVerify(exactly = 0) { commands.listPending(any(), any()) }
    }

    @Test fun `receipt revision is recorded only for accepted non-delete`() = runTest {
        every { sessions.load() } returns session()
        every { syncScope.isDeviceEnabled() } returns true
        every { syncScope.isDatasetEnabled(any()) } returns true
        coEvery { commands.listPending(any(), any()) } returns listOf(command())
        coEvery { gateway.submit(any(), any(), any(), any(), any()) } returns LedgerSyncReceipt("op-1", 7, false)
        assertEquals(LedgerDispatchOutcome.DONE, gated().invoke(null))
        coVerify(exactly = 1) { acks.recordAck("acct-1", scope.origin, "refuel", 7, 1000L) }
    }

    @Test fun `accepted delete never records backup ack`() = runTest {
        every { sessions.load() } returns session()
        every { syncScope.isDeviceEnabled() } returns true
        every { syncScope.isDatasetEnabled(any()) } returns true
        coEvery { commands.listPending(any(), any()) } returns listOf(command().copy(delete = true))
        coEvery { gateway.submit(any(), any(), any(), any(), any()) } returns LedgerSyncReceipt("op-1", 8, false)
        assertEquals(LedgerDispatchOutcome.DONE, gated().invoke(null))
        coVerify(exactly = 0) { acks.recordAck(any(), any(), any(), any(), any()) }
    }

    @Test fun `requested command cannot bypass revoked dataset`() = runTest {
        every { sessions.load() } returns session()
        every { syncScope.isDeviceEnabled() } returns true
        every { syncScope.isDatasetEnabled(any()) } returns false
        coEvery { commands.findPending(any(), "op-1") } returns command()
        assertEquals(LedgerDispatchOutcome.DONE, gated().invoke("op-1"))
        coVerify(exactly = 0) { gateway.submit(any(), any(), any(), any(), any()) }
    }

    @Test fun `dataset revoked while claim suspends prevents transport`() = runTest {
        every { sessions.load() } returns session()
        every { syncScope.isDeviceEnabled() } returns true
        every { syncScope.isDatasetEnabled(any()) } returns true
        coEvery { commands.listPending(any(), any()) } returns listOf(command())
        coEvery { commands.claim(any(), any(), any(), any()) } answers {
            every { syncScope.isDatasetEnabled("refuel") } returns false
            true
        }
        assertEquals(LedgerDispatchOutcome.DONE, gated().invoke(null))
        coVerify(exactly = 0) { gateway.submit(any(), any(), any(), any(), any()) }
        coVerify(exactly = 0) { commands.acknowledge(any(), any()) }
    }

    @Test fun `owner switched during claim never sends the old proof`() = runTest {
        every { sessions.load() } returns session()
        every { syncScope.isDeviceEnabled() } returns true
        every { syncScope.isDatasetEnabled(any()) } returns true
        coEvery { commands.listPending(any(), any()) } returns listOf(command())
        coEvery { commands.claim(any(), any(), any(), any()) } answers {
            every { sessions.load() } returns session().copy(accountId = "other")
            true
        }
        assertEquals(LedgerDispatchOutcome.REFUSED, gated().invoke(null))
        coVerify(exactly = 0) { gateway.submit(any(), any(), any(), any(), any()) }
    }

    @Test fun `obsolete queued body is held rather than sent over a local correction`() = runTest {
        every { sessions.load() } returns session()
        coEvery { commands.listPending(any(),any()) } returns listOf(command())
        val drafts = mockk<com.anpfuel.domain.repository.LedgerDraftStore>()
        coEvery { drafts.find(any(),any(),any(),any()) } returns null
        val guarded = DispatchLedgerCommandUseCase(commands,gateway,sessions,"https://api.example.test", drafts=drafts)
        assertEquals(LedgerDispatchOutcome.REFUSED,guarded.invoke(null))
        coVerify(exactly=0) { gateway.submit(any(),any(),any(),any(),any()) }
        coVerify(exactly=1) { commands.markConflict(scope,"op-1","ledger.local-record-changed") }
        coVerify(exactly=0) { commands.acknowledge(any(),any()) }
    }

    private fun currentDraft() = com.anpfuel.domain.repository.StoredLedgerDraft(
        com.anpfuel.domain.premium.RefuelDraft(
            "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222",
            Instant.parse("2026-03-01T12:00:00Z"), "UTC", com.anpfuel.domain.premium.PremiumFuel.ETHANOL,
            40000, 26000, null, 10000000, com.anpfuel.domain.premium.LedgerFill.FULL, null, null,
        ), 0, 1,
    )

    private suspend fun pausedDraftRead(
        revoke: () -> Unit, expected: LedgerDispatchOutcome,
    ) {
        val stored = currentDraft()
        val queued = command().copy(recordId = stored.draft.recordId, payloadJson = stored.draft.bodyJson())
        val drafts = mockk<com.anpfuel.domain.repository.LedgerDraftStore>()
        val entered = CompletableDeferred<Unit>()
        val resume = CompletableDeferred<Unit>()
        every { sessions.load() } returns session()
        every { syncScope.isDeviceEnabled() } returns true
        every { syncScope.isDatasetEnabled(any()) } returns true
        coEvery { commands.listPending(scope, any()) } returns listOf(queued)
        coEvery { drafts.find(scope.accountId, scope.origin, LedgerDataset.REFUEL, queued.recordId) } coAnswers {
            entered.complete(Unit)
            resume.await()
            stored
        }
        coEvery { gateway.submit(any(), any(), any(), any(), any()) } returns LedgerSyncReceipt("op-1", 1, false)
        val guarded = DispatchLedgerCommandUseCase(commands, gateway, sessions, scope.origin,
            syncScope = syncScope, acks = acks, drafts = drafts)
        kotlinx.coroutines.coroutineScope {
            val dispatch = async { guarded.invoke(null) }
            entered.await()
            revoke()
            resume.complete(Unit)
            assertEquals(expected, dispatch.await())
        }
        coVerify(exactly = 0) { gateway.submit(any(), any(), any(), any(), any()) }
        coVerify(exactly = 0) { commands.acknowledge(any(), any()) }
        coVerify(exactly = 0) { commands.markConflict(any(), any(), any()) }
        coVerify(exactly = 0) { acks.recordAck(any(), any(), any(), any(), any()) }
    }

    @Test fun `owner switched while current draft read suspends prevents transport`() = runTest {
        pausedDraftRead({ every { sessions.load() } returns session().copy(accountId = "other") }, LedgerDispatchOutcome.REFUSED)
    }

    @Test fun `same owner new login while current draft read suspends refuses old family`() = runTest {
        pausedDraftRead({ every { sessions.load() } returns session().copy(familyId = "new-family") }, LedgerDispatchOutcome.REFUSED)
    }

    @Test fun `device opt out while current draft read suspends prevents transport`() = runTest {
        pausedDraftRead({ every { syncScope.isDeviceEnabled() } returns false }, LedgerDispatchOutcome.DONE)
    }

    @Test fun `dataset opt out while current draft read suspends prevents transport`() = runTest {
        pausedDraftRead({ every { syncScope.isDatasetEnabled("refuel") } returns false }, LedgerDispatchOutcome.DONE)
    }

    @Test fun `same body with a newer local revision holds obsolete queued command`() = runTest {
        val stored = currentDraft()
        val queued = command().copy(recordId = stored.draft.recordId, payloadJson = stored.draft.bodyJson())
        val drafts = mockk<com.anpfuel.domain.repository.LedgerDraftStore>()
        every { sessions.load() } returns session()
        coEvery { commands.listPending(scope, any()) } returns listOf(queued)
        coEvery { drafts.find(any(), any(), any(), any()) } returns stored.copy(expectedRevision = 1)
        coEvery { gateway.submit(any(), any(), any(), any(), any()) } returns LedgerSyncReceipt("op-1", 1, false)
        val guarded = DispatchLedgerCommandUseCase(commands, gateway, sessions, scope.origin, drafts = drafts)
        assertEquals(LedgerDispatchOutcome.REFUSED, guarded.invoke(null))
        coVerify(exactly = 0) { gateway.submit(any(), any(), any(), any(), any()) }
        coVerify(exactly = 1) { commands.markConflict(scope, "op-1", "ledger.local-record-changed") }
    }

    @Test fun `matching current draft sends once with refreshed token in the same family`() = runTest {
        val stored = currentDraft()
        val queued = command().copy(recordId = stored.draft.recordId, payloadJson = stored.draft.bodyJson())
        val drafts = mockk<com.anpfuel.domain.repository.LedgerDraftStore>()
        every { sessions.load() } returns session()
        coEvery { commands.listPending(scope, any()) } returns listOf(queued)
        coEvery { drafts.find(scope.accountId, scope.origin, LedgerDataset.REFUEL, queued.recordId) } coAnswers {
            every { sessions.load() } returns session().copy(accessToken = "refreshed-synthetic-token")
            stored
        }
        coEvery { gateway.submit(scope, queued, any(), "fam-1", "refreshed-synthetic-token") } returns LedgerSyncReceipt("op-1", 1, false)
        val guarded = DispatchLedgerCommandUseCase(commands, gateway, sessions, scope.origin, drafts = drafts)
        assertEquals(LedgerDispatchOutcome.DONE, guarded.invoke(null))
        coVerify(exactly = 1) { gateway.submit(scope, queued, any(), "fam-1", "refreshed-synthetic-token") }
        coVerify(exactly = 1) { commands.acknowledge(scope, "op-1") }
        coVerify(exactly = 0) { commands.markConflict(any(), any(), any()) }
    }

    @Test fun `gap and reset intents still dispatch when the draft store guard is enabled`() = runTest {
        val vehicle = "22222222-2222-4222-8222-222222222222"
        val queued = listOf(
            com.anpfuel.domain.premium.LedgerIntentCommandFactory.gapCommand(scope, vehicle,
                com.anpfuel.domain.premium.LedgerGapIntent("11111111-1111-4111-8111-111111111111", null, null)),
            com.anpfuel.domain.premium.LedgerIntentCommandFactory.resetCommand(scope, vehicle,
                com.anpfuel.domain.premium.LedgerSegmentReset("replacement")),
        )
        val drafts = mockk<com.anpfuel.domain.repository.LedgerDraftStore>()
        every { sessions.load() } returns session()
        val guarded = DispatchLedgerCommandUseCase(commands, gateway, sessions, scope.origin, drafts = drafts)
        for (intent in queued) {
            coEvery { commands.listPending(scope, any()) } returns listOf(intent)
            coEvery { gateway.submit(scope, intent, any(), "fam-1", "at") } returns LedgerSyncReceipt(intent.operationId, 1, false)
            assertEquals(LedgerDispatchOutcome.DONE, guarded.invoke(null), intent.dataset)
            coVerify(exactly = 1) { gateway.submit(scope, intent, any(), "fam-1", "at") }
            coVerify(exactly = 1) { commands.acknowledge(scope, intent.operationId) }
        }
        coVerify(exactly = 0) { drafts.find(any(), any(), any(), any()) }
        coVerify(exactly = 0) { commands.markConflict(any(), any(), any()) }
    }

}
