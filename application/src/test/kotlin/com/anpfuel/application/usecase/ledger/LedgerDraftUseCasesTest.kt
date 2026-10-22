package com.anpfuel.application.usecase.ledger

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.premium.ExpenseCategory
import com.anpfuel.domain.premium.ExpenseDraft
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.PremiumCommand
import com.anpfuel.domain.premium.PremiumOwnerScope
import com.anpfuel.domain.premium.PremiumValidationException
import com.anpfuel.domain.repository.LedgerDraftStore
import com.anpfuel.domain.repository.PremiumCommandStore
import com.anpfuel.domain.repository.StoredLedgerDraft
import io.mockk.every
import io.mockk.mockk
import java.time.Instant
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class LedgerDraftUseCasesTest {

    private val at = Instant.parse("2026-03-15T10:30:00Z")
    private val vid = "22222222-2222-4222-8222-222222222222"
    private val id = "11111111-1111-4111-8111-111111111111"

    private class FakeDrafts : LedgerDraftStore {
        val rows = mutableMapOf<String, StoredLedgerDraft>()
        override suspend fun save(accountId: String, origin: String, draft: com.anpfuel.domain.premium.LedgerDraft, expectedRevision: Long, nowMillis: Long) =
            StoredLedgerDraft(draft, expectedRevision, nowMillis).also { rows[draft.recordId] = it }
        override suspend fun find(accountId: String, origin: String, dataset: LedgerDataset, recordId: String) = rows[recordId]
        override suspend fun list(accountId: String, origin: String) = rows.values.toList()
        override suspend fun delete(accountId: String, origin: String, dataset: LedgerDataset, recordId: String) {
            rows.remove(recordId)
        }
        override suspend fun clearOwner(accountId: String, origin: String) {
            rows.clear()
        }
    }

    private class FakeCommands : PremiumCommandStore {
        val rows = mutableMapOf<String, PremiumCommand>()
        var releaseResult = 0
        var releaseCalledWith: Long? = null
        override suspend fun enqueue(command: PremiumCommand) {
            rows.putIfAbsent(command.operationId, command)
        }
        override suspend fun findPending(scope: PremiumOwnerScope, operationId: String) = rows[operationId]
        override suspend fun listPending(scope: PremiumOwnerScope, limit: Int) = rows.values.toList()
        override suspend fun remove(scope: PremiumOwnerScope, operationId: String) {
            rows.remove(operationId)
        }
        override suspend fun clearOwner(scope: PremiumOwnerScope) {
            rows.clear()
        }
        override suspend fun releaseExpired(nowMillis: Long): Int {
            releaseCalledWith = nowMillis
            return releaseResult
        }
    }

    private val drafts = FakeDrafts()
    private val commands = FakeCommands()
    private val sessions = mockk<AuthSessionStore>()

    private lateinit var save: SaveLedgerDraftUseCase
    private lateinit var enqueue: EnqueueLedgerCommandUseCase
    private lateinit var recover: RecoverLedgerOutboxUseCase

    @BeforeEach
    fun setUp() {
        every { sessions.load() } returns null
        save = SaveLedgerDraftUseCase(drafts, sessions, "https://api.example.test", nowMillis = { 1L })
        enqueue = EnqueueLedgerCommandUseCase(drafts, commands, sessions, "https://api.example.test")
        recover = RecoverLedgerOutboxUseCase(commands, nowMillis = { 2L })
    }

    private fun expense() = ExpenseDraft(id, vid, at, "UTC", 5000, ExpenseCategory.TOLLS, null, null)

    @Test fun `double save converges and enqueue is idempotent`() = runTest {
        val first = save(expense(), 0)
        val second = save(expense(), 0)
        assertEquals(first.draft.bodyJson(), second.draft.bodyJson())
        assertEquals(1, drafts.rows.size)
        val one = enqueue(LedgerDataset.EXPENSE, id)
        val two = enqueue(LedgerDataset.EXPENSE, id)
        assertEquals(one.operationId, two.operationId)
        assertEquals(1, commands.rows.size)
        // A correction carries a new revision and a new operation id.
        save(expense(), 1)
        val corrected = enqueue(LedgerDataset.EXPENSE, id)
        assertTrue(corrected.operationId != one.operationId)
        assertEquals(2, commands.rows.size)
    }

    @Test fun `invalid drafts and missing rows refuse without writes`() = runTest {
        val badRevision = try {
            save(expense(), -1)
            null
        } catch (e: PremiumValidationException) {
            e.code
        }
        assertEquals("invalid-revision", badRevision)
        val missing = try {
            enqueue(LedgerDataset.EXPENSE, "missing")
            null
        } catch (e: PremiumValidationException) {
            e.code
        }
        assertEquals("draft-missing", missing)
        assertTrue(commands.rows.isEmpty())
    }

    @Test fun `guest drafts stay local and sessions scope owners`() = runTest {
        every { sessions.load() } returns PortableAuth.Session("fam-1", "acct-9", "a", "r", 0, 0)
        save(expense(), 0)
        val queued = enqueue(LedgerDataset.EXPENSE, id)
        assertEquals("acct-9", queued.scope.accountId)
    }

    @Test fun `recovery releases crashed claims with the process clock`() = runTest {
        commands.releaseResult = 3
        assertEquals(3, recover())
        assertEquals(2L, commands.releaseCalledWith)
    }
}
