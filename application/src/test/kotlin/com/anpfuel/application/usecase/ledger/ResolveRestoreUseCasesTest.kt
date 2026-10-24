package com.anpfuel.application.usecase.ledger

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.LedgerFill
import com.anpfuel.domain.premium.PremiumCommand
import com.anpfuel.domain.premium.PremiumFuel
import com.anpfuel.domain.premium.PremiumOwnerScope
import com.anpfuel.domain.premium.RefuelDraft
import com.anpfuel.domain.repository.LedgerDraftStore
import com.anpfuel.domain.repository.LedgerErasureLog
import com.anpfuel.domain.repository.PremiumCommandStore
import com.anpfuel.domain.repository.StoredLedgerDraft
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import java.time.Instant
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class ResolveRestoreUseCasesTest {

    private val at = Instant.parse("2026-03-15T10:30:00Z")
    private val vid = "22222222-2222-4222-8222-222222222222"
    private val id = "11111111-1111-4111-8111-111111111111"
    private val scope = PremiumOwnerScope("acct-1", "https://api.example.test")

    private class FakeDrafts : LedgerDraftStore {
        val rows = mutableMapOf<String, StoredLedgerDraft>()
        override suspend fun save(accountId: String, origin: String, draft: com.anpfuel.domain.premium.LedgerDraft, expectedRevision: Long, nowMillis: Long, context: com.anpfuel.domain.premium.LedgerSessionContext?) =
            StoredLedgerDraft(draft, expectedRevision, nowMillis, context).also { rows["$accountId|$origin|${draft.dataset.wire}|${draft.recordId}"] = it }
        override suspend fun find(accountId: String, origin: String, dataset: LedgerDataset, recordId: String) = rows["$accountId|$origin|${dataset.wire}|$recordId"]
        override suspend fun list(accountId: String, origin: String) = rows.filterKeys { it.startsWith("$accountId|$origin|") }.values.toList()
        override suspend fun delete(accountId: String, origin: String, dataset: LedgerDataset, recordId: String) {
            rows.remove("$accountId|$origin|${dataset.wire}|$recordId")
        }
        override suspend fun clearOwner(accountId: String, origin: String) {
            rows.keys.removeAll { it.startsWith("$accountId|$origin|") }
        }
    }

    private class FakeCommands : PremiumCommandStore {
        val rows = mutableMapOf<String, PremiumCommand>()
        val removed = mutableListOf<String>()
        val conflicted = mutableSetOf<String>()
        override suspend fun enqueue(command: PremiumCommand) {
            rows.putIfAbsent(command.operationId, command)
        }
        override suspend fun findPending(scope: PremiumOwnerScope, operationId: String) = rows[operationId]
        override suspend fun listPending(scope: PremiumOwnerScope, limit: Int) = rows.values.toList()
        override suspend fun remove(scope: PremiumOwnerScope, operationId: String) {
            rows.remove(operationId)
            removed.add(operationId)
        }
        override suspend fun clearOwner(scope: PremiumOwnerScope) { rows.clear() }
        override suspend fun releaseExpired(nowMillis: Long) = 0
        override suspend fun claim(operationId: String, scope: PremiumOwnerScope, nonce: String, leaseUntilMillis: Long) = rows.containsKey(operationId)
        override suspend fun failTransient(scope: PremiumOwnerScope, operationId: String, nonce: String, nowMillis: Long) = Unit
        override suspend fun markConflict(scope: PremiumOwnerScope, operationId: String, reason: String) = Unit
        override suspend fun acknowledge(scope: PremiumOwnerScope, operationId: String) {
            rows.remove(operationId)
        }
        override suspend fun hasHeldRecord(scope: com.anpfuel.domain.premium.PremiumOwnerScope, dataset: String, recordId: String) =
            rows.values.any { it.scope == scope && it.dataset == dataset && it.recordId == recordId }
        override suspend fun removeRecord(scope: com.anpfuel.domain.premium.PremiumOwnerScope, dataset: String, recordId: String) {
            rows.values.filter { it.scope == scope && it.dataset == dataset && it.recordId == recordId }.toList().forEach { remove(scope, it.operationId) }
        }
        override suspend fun countPending(scope: PremiumOwnerScope, limit: Int, dataset: String?) = rows.values.count { it.scope == scope && (dataset == null || it.dataset == dataset) }.coerceAtMost(limit)
        override suspend fun listConflicts(scope: PremiumOwnerScope, limit: Int) = rows.values.filter { it.operationId in conflicted && it.scope == scope }.take(limit)
    }

    private class FakeErasures : LedgerErasureLog {
        val erased = mutableSetOf<String>()
        override suspend fun recordErased(accountId: String, origin: String, dataset: String, recordId: String, nowMillis: Long) {
            erased.add("$accountId|$origin|$dataset|$recordId")
        }
        override suspend fun isErased(accountId: String, origin: String, dataset: String, recordId: String) =
            "$accountId|$origin|$dataset|$recordId" in erased
        override suspend fun clearOwner(accountId: String, origin: String) {
            erased.clear()
        }
    }

    private class FakeDecoder : com.anpfuel.domain.repository.LedgerDraftDecoder {
        val bodies = mutableMapOf<String, com.anpfuel.domain.premium.LedgerDraft>()
        override fun decode(dataset: LedgerDataset, bodyJson: String): com.anpfuel.domain.premium.LedgerDraft =
            bodies[bodyJson] ?: throw com.anpfuel.domain.premium.PremiumValidationException("corrupt-payload")
    }

    private val drafts = FakeDrafts()
    private val decoder = FakeDecoder()
    private val commands = FakeCommands()
    private val erasures = FakeErasures()
    private val gateway = mockk<com.anpfuel.domain.repository.LedgerSyncGateway>()
    private val records = mockk<com.anpfuel.domain.repository.LedgerRecordsGateway>()
    private val sessions = mockk<AuthSessionStore>()

    private fun refuel() = RefuelDraft(id, vid, at, "UTC", PremiumFuel.ETHANOL, 20000, 12400, null, 100000, LedgerFill.FULL, null, null)

    private fun session() = PortableAuth.Session("fam-1", "acct-1", "tok", "tok", Long.MAX_VALUE, Long.MAX_VALUE)

    private fun serverRecord(revision: Long, body: String) = com.anpfuel.domain.repository.LedgerServerRecord(
        dataset = "refuel", id = id, vehicleId = vid, occurredAt = "2026-03-15T10:30:00Z",
        revision = revision, voided = false, payloadJson = body,
    )

    @BeforeEach
    fun setUp() {
        every { sessions.load() } returns session()
    }

    @Test fun `keep-mine rebases onto the server revision without duplicating`() = runTest {
        drafts.save("acct-1", "https://api.example.test", refuel(), 0, 1L, null)
        val conflicted = com.anpfuel.domain.premium.LedgerCommandFactory.toCommand(scope, refuel(), 0)
        commands.rows[conflicted.operationId] = conflicted
        coEvery { records.list(any(), any(), any(), any(), 100, null) } returns
            com.anpfuel.domain.repository.LedgerRecordsPage(listOf(serverRecord(2, refuel().bodyJson())), null)
        val resolve = ResolveLedgerConflictUseCase(drafts, commands, records, sessions, "https://api.example.test", erasures, decoder, nowMillis = { 9L })
        val out = resolve(LedgerDataset.REFUEL, id, LedgerConflictChoice.KEEP_MINE)
        assertTrue(out is ResolveOutcome.AppliedKeepMine && out.newRevision == 2L)
        assertEquals(2L, drafts.find("acct-1", "https://api.example.test", LedgerDataset.REFUEL, id)!!.expectedRevision)
        assertTrue(commands.removed.contains(conflicted.operationId))
        assertTrue(commands.rows.values.any { it.expectedRevision == 2L })
    }

    @Test fun `take-server adopts server state and clears the conflict`() = runTest {
        drafts.save("acct-1", "https://api.example.test", refuel(), 0, 1L, null)
        decoder.bodies[refuel().bodyJson()] = refuel()
        val conflicted = com.anpfuel.domain.premium.LedgerCommandFactory.toCommand(scope, refuel(), 0)
        commands.rows[conflicted.operationId] = conflicted
        coEvery { records.list(any(), any(), any(), any(), 100, null) } returns
            com.anpfuel.domain.repository.LedgerRecordsPage(listOf(serverRecord(3, refuel().bodyJson())), null)
        val resolve = ResolveLedgerConflictUseCase(drafts, commands, records, sessions, "https://api.example.test", erasures, decoder, nowMillis = { 9L })
        val out = resolve(LedgerDataset.REFUEL, id, LedgerConflictChoice.TAKE_SERVER)
        assertTrue(out is ResolveOutcome.AppliedTakeServer)
        assertEquals(3L, drafts.find("acct-1", "https://api.example.test", LedgerDataset.REFUEL, id)!!.expectedRevision)
        assertTrue(commands.removed.contains(conflicted.operationId))
    }

    @Test fun `cancel keeps held work and server-missing take-server drops locally`() = runTest {
        drafts.save("acct-1", "https://api.example.test", refuel(), 0, 1L, null)
        coEvery { records.list(any(), any(), any(), any(), 100, null) } returns
            com.anpfuel.domain.repository.LedgerRecordsPage(emptyList(), null)
        val resolve = ResolveLedgerConflictUseCase(drafts, commands, records, sessions, "https://api.example.test", erasures, decoder, nowMillis = { 9L })
        assertTrue(resolve(LedgerDataset.REFUEL, id, LedgerConflictChoice.CANCEL) is ResolveOutcome.Cancelled)
        assertTrue(drafts.find("acct-1", "https://api.example.test", LedgerDataset.REFUEL, id) != null)
        assertTrue(resolve(LedgerDataset.REFUEL, id, LedgerConflictChoice.TAKE_SERVER) is ResolveOutcome.AppliedTakeServer)
        assertTrue(drafts.find("acct-1", "https://api.example.test", LedgerDataset.REFUEL, id) == null)
    }

    @Test fun `restore preview preserves held local work and confirm imports missing records`() = runTest {
        val other = "33333333-3333-4333-8333-333333333333"
        val serverBody = RefuelDraft(other, vid, at, "UTC", PremiumFuel.ETHANOL, 10000, 6000, null, 50000, LedgerFill.FULL, null, null).bodyJson()
        decoder.bodies[serverBody] = RefuelDraft(other, vid, at, "UTC", PremiumFuel.ETHANOL, 10000, 6000, null, 50000, LedgerFill.FULL, null, null)
        decoder.bodies[refuel().bodyJson()] = refuel()
        decoder.bodies[serverBody] = RefuelDraft(other, vid, at, "UTC", PremiumFuel.ETHANOL, 10000, 6000, null, 50000, LedgerFill.FULL, null, null)
        coEvery { records.list(any(), any(), "refuel", any(), 100, null) } returns
            com.anpfuel.domain.repository.LedgerRecordsPage(
                listOf(
                    com.anpfuel.domain.repository.LedgerServerRecord("refuel", other, vid, "2026-03-15T10:30:00Z", 1, false, serverBody),
                    serverRecord(5, refuel().bodyJson()),
                ),
                null,
            )
        coEvery { records.list(any(), any(), "expense", any(), 100, null) } returns
            com.anpfuel.domain.repository.LedgerRecordsPage(emptyList(), null)
        coEvery { records.list(any(), any(), "charge", any(), 100, null) } returns
            com.anpfuel.domain.repository.LedgerRecordsPage(emptyList(), null)
        drafts.save("acct-1", "https://api.example.test", refuel(), 0, 1L, null)
        val preview = PreviewLedgerRestoreUseCase(drafts, erasures, records, sessions, "https://api.example.test")
        val diff = preview()
        assertTrue(diff.toPull.any { it.recordId == other })
        assertTrue(drafts.find("acct-1", "https://api.example.test", LedgerDataset.REFUEL, other) == null)
        val confirm = ConfirmLedgerRestoreUseCase(drafts, erasures, records, sessions, "https://api.example.test", decoder, nowMillis = System::currentTimeMillis)
        val applied = confirm(diff)
        assertTrue(applied is RestoreOutcome.Applied)
        assertTrue((applied as RestoreOutcome.Applied).items.all { it.outcome == RestoreItemOutcome.APPLIED })
        assertTrue(drafts.find("acct-1", "https://api.example.test", LedgerDataset.REFUEL, other) != null)
    }

    @Test fun `restore skips erased rows and aborts on moved server revision`() = runTest {
        val other = "33333333-3333-4333-8333-333333333333"
        val kept = "44444444-4444-4444-8444-444444444444"
        erasures.recordErased("acct-1", "https://api.example.test", "refuel", other, 1L)
        val erasedBody = RefuelDraft(other, vid, at, "UTC", PremiumFuel.ETHANOL, 10000, 6000, null, 50000, LedgerFill.FULL, null, null).bodyJson()
        val keptBody = RefuelDraft(kept, vid, at, "UTC", PremiumFuel.ETHANOL, 20000, 12400, null, 60000, LedgerFill.FULL, null, null).bodyJson()
        coEvery { records.list(any(), any(), "refuel", any(), 100, null) } returns
            com.anpfuel.domain.repository.LedgerRecordsPage(
                listOf(
                    com.anpfuel.domain.repository.LedgerServerRecord("refuel", other, vid, "2026-03-15T10:30:00Z", 1, false, erasedBody),
                    com.anpfuel.domain.repository.LedgerServerRecord("refuel", kept, vid, "2026-03-15T10:30:00Z", 1, false, keptBody),
                ),
                null,
            ) andThen
            com.anpfuel.domain.repository.LedgerRecordsPage(
                listOf(
                    com.anpfuel.domain.repository.LedgerServerRecord("refuel", other, vid, "2026-03-15T10:30:00Z", 1, false, erasedBody),
                    com.anpfuel.domain.repository.LedgerServerRecord("refuel", kept, vid, "2026-03-15T10:30:00Z", 2, false, keptBody),
                ),
                null,
            )
        coEvery { records.list(any(), any(), "expense", any(), 100, null) } returns
            com.anpfuel.domain.repository.LedgerRecordsPage(emptyList(), null)
        coEvery { records.list(any(), any(), "charge", any(), 100, null) } returns
            com.anpfuel.domain.repository.LedgerRecordsPage(emptyList(), null)
        val preview = PreviewLedgerRestoreUseCase(drafts, erasures, records, sessions, "https://api.example.test")
        val diff = preview()
        assertTrue(diff.toPull.none { it.recordId == other })
        assertTrue(diff.toPull.any { it.recordId == kept })
        val confirm = ConfirmLedgerRestoreUseCase(drafts, erasures, records, sessions, "https://api.example.test", decoder, nowMillis = System::currentTimeMillis)
        assertTrue(confirm(diff) is RestoreOutcome.StaleSnapshot)
        assertTrue(drafts.find("acct-1", "https://api.example.test", LedgerDataset.REFUEL, other) == null)
        assertTrue(drafts.find("acct-1", "https://api.example.test", LedgerDataset.REFUEL, kept) == null)
    }

    @Test fun `account switch partitions restore and clearOwner wipes erasures`() = runTest {
        coEvery { records.list(any(), any(), "refuel", any(), 100, null) } returns
            com.anpfuel.domain.repository.LedgerRecordsPage(emptyList(), null)
        coEvery { records.list(any(), any(), "expense", any(), 100, null) } returns
            com.anpfuel.domain.repository.LedgerRecordsPage(emptyList(), null)
        coEvery { records.list(any(), any(), "charge", any(), 100, null) } returns
            com.anpfuel.domain.repository.LedgerRecordsPage(emptyList(), null)
        val preview = PreviewLedgerRestoreUseCase(drafts, erasures, records, sessions, "https://api.example.test")
        preview()
        erasures.clearOwner("acct-1", "https://api.example.test")
        assertTrue(erasures.erased.isEmpty())
    }

    @Test fun `offline take-server preserves local draft and queue`() = runTest {
        drafts.save("acct-1", scope.origin, refuel(), 0, 1L, null)
        val cmd = com.anpfuel.domain.premium.LedgerCommandFactory.toCommand(scope, refuel(), 0)
        commands.rows[cmd.operationId] = cmd
        coEvery { records.list(any(), any(), any(), any(), 100, null) } throws java.io.IOException("offline")
        val resolve = ResolveLedgerConflictUseCase(drafts, commands, records, sessions, scope.origin, erasures, decoder)
        try { resolve(LedgerDataset.REFUEL, id, LedgerConflictChoice.TAKE_SERVER) } catch (_: java.io.IOException) { }
        assertTrue(drafts.rows.values.any { it.draft.recordId == id })
        assertTrue(commands.rows.containsKey(cmd.operationId))
        assertTrue(erasures.erased.isEmpty())
    }

    @Test fun `preview from previous account cannot write into current account`() = runTest {
        coEvery { records.list(any(), any(), "refuel", null, 100, null) } returns
            com.anpfuel.domain.repository.LedgerRecordsPage(listOf(serverRecord(1, refuel().bodyJson())), null)
        coEvery { records.list(any(), any(), "expense", null, 100, null) } returns com.anpfuel.domain.repository.LedgerRecordsPage(emptyList(), null)
        coEvery { records.list(any(), any(), "charge", null, 100, null) } returns com.anpfuel.domain.repository.LedgerRecordsPage(emptyList(), null)
        decoder.bodies[refuel().bodyJson()] = refuel()
        val diff = PreviewLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin)()
        every { sessions.load() } returns session().copy(accountId = "acct-2")
        val result = ConfirmLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin, decoder)(diff)
        assertEquals(RestoreOutcome.StaleSnapshot, result)
        assertTrue(drafts.rows.isEmpty())
    }

    private fun source() {
        decoder.bodies[refuel().bodyJson()] = refuel()
        coEvery { records.list(any(), any(), "refuel", null, 100, null) } returns
            com.anpfuel.domain.repository.LedgerRecordsPage(listOf(serverRecord(1, refuel().bodyJson())), null)
        coEvery { records.list(any(), any(), "expense", null, 100, null) } returns com.anpfuel.domain.repository.LedgerRecordsPage(emptyList(), null)
        coEvery { records.list(any(), any(), "charge", null, 100, null) } returns com.anpfuel.domain.repository.LedgerRecordsPage(emptyList(), null)
    }

    @Test fun `restore confirmation is idempotent and preserves original local write time`() = runTest {
        source()
        val diff = PreviewLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin)()
        val confirm = ConfirmLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin, decoder)
        assertTrue(confirm(diff) is RestoreOutcome.Applied)
        val before = drafts.rows.toMap()
        assertTrue(confirm(diff) is RestoreOutcome.Applied)
        assertEquals(before, drafts.rows)
    }

    @Test fun `erase after preview prevents resurrection even when old server snapshot is unchanged`() = runTest {
        source()
        val diff = PreviewLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin)()
        erasures.recordErased("acct-1", scope.origin, "refuel", id, 1)
        val result = ConfirmLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin, decoder)(diff) as RestoreOutcome.Applied
        assertEquals(RestoreItemOutcome.SKIPPED_ERASED, result.items.single().outcome)
        assertTrue(drafts.rows.isEmpty())
    }

    @Test fun `new local edit after preview aborts the batch before writes`() = runTest {
        source()
        val diff = PreviewLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin)()
        drafts.save("acct-1", scope.origin, refuel(), 0, 1, null)
        val before = drafts.rows.toMap()
        assertEquals(RestoreOutcome.StaleSnapshot, ConfirmLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin, decoder)(diff))
        assertEquals(before, drafts.rows)
    }

    @Test fun `expired truncated and revoked previews abort without local writes`() = runTest {
        source()
        val gate = mockk<com.anpfuel.domain.repository.LedgerSyncScope>()
        every { gate.isDeviceEnabled() } returns true
        every { gate.isDatasetEnabled(any()) } returns true
        val diff = PreviewLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin, nowMillis = { 1 })()
        val confirm = ConfirmLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin, decoder, nowMillis = { 2 }, scope = gate)
        assertEquals(RestoreOutcome.StaleSnapshot, confirm(diff.copy(createdAtMillis = -RESTORE_PREVIEW_TTL_MILLIS)))
        assertEquals(RestoreOutcome.StaleSnapshot, confirm(diff.copy(truncated = true)))
        every { gate.isDatasetEnabled("refuel") } returns false
        assertEquals(RestoreOutcome.StaleSnapshot, confirm(diff))
        assertTrue(drafts.rows.isEmpty())
    }

    @Test fun `corrupt item is reported without discarding other valid imports`() = runTest {
        source()
        val bad = "33333333-3333-4333-8333-333333333333"
        coEvery { records.list(any(), any(), "refuel", null, 100, null) } returns
            com.anpfuel.domain.repository.LedgerRecordsPage(listOf(serverRecord(1, refuel().bodyJson()), serverRecord(1, "corrupt").copy(id = bad)), null)
        val diff = PreviewLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin)()
        val result = ConfirmLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin, decoder)(diff) as RestoreOutcome.Applied
        assertEquals(listOf(RestoreItemOutcome.APPLIED, RestoreItemOutcome.UNDECODABLE), result.items.map { it.outcome })
        assertEquals(1, drafts.rows.size)
    }

    @Test fun `offline preview propagates instead of offering a misleading partial restore`() = runTest {
        source()
        coEvery { records.list(any(), any(), "expense", null, 100, null) } throws java.io.IOException("offline")
        org.junit.jupiter.api.Assertions.assertThrows(java.io.IOException::class.java) {
            kotlinx.coroutines.runBlocking { PreviewLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin)() }
        }
        assertTrue(drafts.rows.isEmpty())
    }

    @Test fun `preview cancellation propagates without writing`() = runTest {
        source()
        coEvery { records.list(any(), any(), "refuel", null, 100, null) } throws kotlinx.coroutines.CancellationException("cancel")
        org.junit.jupiter.api.Assertions.assertThrows(kotlinx.coroutines.CancellationException::class.java) {
            kotlinx.coroutines.runBlocking { PreviewLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin)() }
        }
        assertTrue(drafts.rows.isEmpty())
    }

    @Test fun `conflict lookup follows cursor before interpreting a record as missing`() = runTest {
        drafts.save("acct-1", scope.origin, refuel(), 0, 1, null)
        coEvery { records.list(any(), any(), "refuel", vid, 100, null) } returns com.anpfuel.domain.repository.LedgerRecordsPage(emptyList(), "next")
        coEvery { records.list(any(), any(), "refuel", vid, 100, "next") } returns com.anpfuel.domain.repository.LedgerRecordsPage(listOf(serverRecord(9, refuel().bodyJson())), null)
        val resolve = ResolveLedgerConflictUseCase(drafts, commands, records, sessions, scope.origin, erasures, decoder)
        assertEquals(ResolveOutcome.AppliedKeepMine(9), resolve(LedgerDataset.REFUEL, id, LedgerConflictChoice.KEEP_MINE))
    }

    @Test fun `owner change during conflict lookup preserves draft and command`() = runTest {
        drafts.save("acct-1", scope.origin, refuel(), 0, 1, null)
        coEvery { records.list(any(), any(), "refuel", vid, 100, null) } answers {
            every { sessions.load() } returns session().copy(accountId = "acct-2")
            com.anpfuel.domain.repository.LedgerRecordsPage(emptyList(), null)
        }
        val resolve = ResolveLedgerConflictUseCase(drafts, commands, records, sessions, scope.origin, erasures, decoder)
        org.junit.jupiter.api.Assertions.assertThrows(IllegalStateException::class.java) {
            kotlinx.coroutines.runBlocking { resolve(LedgerDataset.REFUEL, id, LedgerConflictChoice.TAKE_SERVER) }
        }
        assertEquals(1, drafts.list("acct-1", scope.origin).size)
        assertTrue(drafts.list("acct-2", scope.origin).isEmpty())
    }

    @Test fun `local delete records durable erasure before server ack`() = runTest {
        drafts.save("acct-1", scope.origin, refuel(), 0, 1, null)
        DeleteLedgerEntryUseCase(drafts, commands, sessions, scope.origin, erasures)(LedgerDataset.REFUEL, id)
        assertTrue(drafts.rows.isEmpty())
        assertTrue(erasures.isErased("acct-1", scope.origin, "refuel", id))
        assertTrue(commands.rows.values.single().delete)
    }

    @Test fun `missing keep mine and corrupt take server retain recoverable local work`() = runTest {
        drafts.save("acct-1", scope.origin, refuel(), 0, 1, null)
        val cmd = com.anpfuel.domain.premium.LedgerCommandFactory.toCommand(scope, refuel(), 0)
        commands.rows[cmd.operationId] = cmd
        coEvery { records.list(any(), any(), "refuel", vid, 100, null) } returns com.anpfuel.domain.repository.LedgerRecordsPage(emptyList(), null)
        val resolve = ResolveLedgerConflictUseCase(drafts, commands, records, sessions, scope.origin, erasures, decoder)
        assertEquals(ResolveOutcome.NoContent, resolve(LedgerDataset.REFUEL, id, LedgerConflictChoice.KEEP_MINE))
        coEvery { records.list(any(), any(), "refuel", vid, 100, null) } returns com.anpfuel.domain.repository.LedgerRecordsPage(listOf(serverRecord(2, "corrupt")), null)
        assertTrue(resolve(LedgerDataset.REFUEL, id, LedgerConflictChoice.TAKE_SERVER) is ResolveOutcome.Undecodable)
        assertEquals(1, drafts.rows.size)
        assertEquals(1, commands.rows.size)
    }

    @Test fun `scope revoked during network revalidation aborts before the local transaction writes`() = runTest {
        source()
        val diff = PreviewLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin)()
        val gate = mockk<com.anpfuel.domain.repository.LedgerSyncScope>()
        var enabled = true
        every { gate.isDeviceEnabled() } returns true
        every { gate.isDatasetEnabled(any()) } answers { enabled }
        coEvery { records.list(any(), any(), "refuel", null, 100, null) } answers {
            enabled = false
            com.anpfuel.domain.repository.LedgerRecordsPage(listOf(serverRecord(1, refuel().bodyJson())), null)
        }
        val confirm = ConfirmLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin, decoder, scope = gate)
        assertEquals(RestoreOutcome.StaleSnapshot, confirm(diff))
        assertTrue(drafts.rows.isEmpty())
    }

    @Test fun `persistence failure rolls back earlier restored items through transaction boundary`() = runTest {
        source()
        val other = refuel().copy(id = "33333333-3333-4333-8333-333333333333")
        decoder.bodies[other.bodyJson()] = other
        coEvery { records.list(any(), any(), "refuel", null, 100, null) } returns
            com.anpfuel.domain.repository.LedgerRecordsPage(listOf(serverRecord(1, refuel().bodyJson()), serverRecord(1, other.bodyJson()).copy(id = other.id)), null)
        val diff = PreviewLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin)()
        val failing = object : LedgerDraftStore by drafts {
            override suspend fun save(accountId: String, origin: String, draft: com.anpfuel.domain.premium.LedgerDraft, expectedRevision: Long, nowMillis: Long, context: com.anpfuel.domain.premium.LedgerSessionContext?): StoredLedgerDraft {
                if (draft.recordId == other.id) throw java.io.IOException("disk unavailable")
                return drafts.save(accountId, origin, draft, expectedRevision, nowMillis, context)
            }
        }
        var transactionEntered = false
        val confirm = ConfirmLedgerRestoreUseCase(failing, erasures, records, sessions, scope.origin, decoder,
            localTransaction = { block ->
                transactionEntered = true
                val before = drafts.rows.toMap()
                try { block() } catch (error: Exception) {
                    drafts.rows.clear(); drafts.rows.putAll(before); throw error
                }
            })
        org.junit.jupiter.api.Assertions.assertThrows(java.io.IOException::class.java) { kotlinx.coroutines.runBlocking { confirm(diff) } }
        assertTrue(transactionEntered)
        assertTrue(drafts.rows.isEmpty())
    }

    @Test fun `pending legacy delete without an erasure marker is not restored`() = runTest {
        source()
        val deleted = com.anpfuel.domain.premium.LedgerCommandFactory.deleteCommand(scope, LedgerDataset.REFUEL, id)
        commands.rows[deleted.operationId] = deleted
        val diff = PreviewLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin, commands = commands)()
        assertTrue(diff.toPull.isEmpty())
        assertTrue(drafts.rows.isEmpty())
        assertEquals(1, commands.rows.size)
    }

    @Test fun `legacy delete queued after preview invalidates confirmation`() = runTest {
        source()
        val diff = PreviewLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin, commands = commands)()
        val deleted = com.anpfuel.domain.premium.LedgerCommandFactory.deleteCommand(scope, LedgerDataset.REFUEL, id)
        commands.rows[deleted.operationId] = deleted
        val result = ConfirmLedgerRestoreUseCase(drafts, erasures, records, sessions, scope.origin, decoder, commands = commands)(diff)
        assertEquals(RestoreOutcome.StaleSnapshot, result)
        assertTrue(drafts.rows.isEmpty())
        assertEquals(1, commands.rows.size)
    }

    @Test fun `delete conflict without a draft can be retried or accepted as server missing`() = runTest {
        val held = com.anpfuel.domain.premium.LedgerCommandFactory.deleteCommand(scope, LedgerDataset.REFUEL, id)
        commands.rows[held.operationId] = held
        commands.conflicted.add(held.operationId)
        coEvery { records.list(any(), any(), "refuel", null, 100, null) } returns
            com.anpfuel.domain.repository.LedgerRecordsPage(listOf(serverRecord(7, refuel().bodyJson())), null)
        val resolve = ResolveLedgerConflictUseCase(drafts, commands, records, sessions, scope.origin, erasures, decoder)
        assertEquals(ResolveOutcome.AppliedKeepMine(7), resolve(LedgerDataset.REFUEL, id, LedgerConflictChoice.KEEP_MINE))
        val retry = commands.rows.values.single()
        assertTrue(retry.delete)
        assertEquals(7L, retry.expectedRevision)
        assertTrue(retry.operationId != held.operationId)
        commands.conflicted.add(retry.operationId)
        coEvery { records.list(any(), any(), "refuel", null, 100, null) } returns com.anpfuel.domain.repository.LedgerRecordsPage(emptyList(), null)
        assertEquals(ResolveOutcome.AppliedTakeServer, resolve(LedgerDataset.REFUEL, id, LedgerConflictChoice.TAKE_SERVER))
        assertTrue(commands.rows.isEmpty())
        assertTrue(erasures.isErased("acct-1", scope.origin, "refuel", id))
    }

}
