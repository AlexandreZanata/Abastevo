package com.anpfuel.application.usecase.ledger

import com.anpfuel.application.portable.AuthSessionStore
import io.mockk.coEvery
import io.mockk.every
import io.mockk.mockk
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.premium.ExpenseCategory
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.LedgerDraft
import com.anpfuel.domain.premium.LedgerFill
import com.anpfuel.domain.premium.PremiumOwnerScope
import com.anpfuel.domain.repository.LedgerDraftStore
import com.anpfuel.domain.repository.PremiumCommandStore
import com.anpfuel.domain.repository.VehicleRepository
import com.anpfuel.domain.valueobject.DomainId
import java.time.Instant
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class LedgerEntriesUseCasesTest {

    private val at = Instant.parse("2026-03-15T10:30:00Z")
    private val vid = "22222222-2222-4222-8222-222222222222"
    private val id = "11111111-1111-4111-8111-111111111111"

    private class FakeDrafts : LedgerDraftStore {
        val rows = mutableMapOf<String, com.anpfuel.domain.repository.StoredLedgerDraft>()
        var now = 1L
        override suspend fun save(accountId: String, origin: String, draft: LedgerDraft, expectedRevision: Long, nowMillis: Long, context: com.anpfuel.domain.premium.LedgerSessionContext?) =
            com.anpfuel.domain.repository.StoredLedgerDraft(draft, expectedRevision, nowMillis).also { rows[draft.recordId] = it }
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
        val rows = mutableMapOf<String, com.anpfuel.domain.premium.PremiumCommand>()
        override suspend fun enqueue(command: com.anpfuel.domain.premium.PremiumCommand) {
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
        override suspend fun releaseExpired(nowMillis: Long) = 0
    }

    private val drafts = FakeDrafts()
    private val commands = FakeCommands()
    private val sessions = mockk<AuthSessionStore>()
    private val vehicles = mockk<VehicleRepository>()

    private lateinit var list: ListLedgerEntriesUseCase
    private lateinit var remove: DeleteLedgerEntryUseCase

    @BeforeEach
    fun setUp() {
        every { sessions.load() } returns null
        list = ListLedgerEntriesUseCase(drafts, commands, sessions, "https://api.example.test", vehicles)
        remove = DeleteLedgerEntryUseCase(drafts, commands, sessions, "https://api.example.test")
    }

    private fun expense(id: String = this.id) = com.anpfuel.domain.premium.ExpenseDraft(
        id, vid, at, "UTC", 5000, ExpenseCategory.TOLLS, null, null,
    )

    @Test fun `draft without queue reads LOCAL and queued same revision reads PENDING`() = runTest {
        coEvery { vehicles.findById(any()) } returns null
        drafts.save("local", "https://api.example.test", expense(), 0, 1)
        assertEquals(LedgerEntryState.LOCAL, list(DomainId.from(vid)).single().state)
        val queued = com.anpfuel.domain.premium.LedgerCommandFactory.toCommand(
            PremiumOwnerScope("local", "https://api.example.test"), expense(), 0,
        )
        commands.enqueue(queued)
        assertEquals(LedgerEntryState.PENDING, list(DomainId.from(vid)).single().state)
    }

    @Test fun `advanced draft against older queue reads CONFLICT`() = runTest {
        coEvery { vehicles.findById(any()) } returns null
        drafts.save("local", "https://api.example.test", expense(), 0, 1)
        commands.enqueue(
            com.anpfuel.domain.premium.LedgerCommandFactory.toCommand(
                PremiumOwnerScope("local", "https://api.example.test"), expense(), 0,
            ),
        )
        drafts.save("local", "https://api.example.test", expense(), 1, 2)
        val row = list(DomainId.from(vid)).single()
        assertEquals(LedgerEntryState.CONFLICT, row.state)
        assertEquals(1L, row.expectedRevision)
    }

    @Test fun `delete drops the draft and queues a tombstone once`() = runTest {
        drafts.save("local", "https://api.example.test", expense(), 0, 1)
        remove(LedgerDataset.EXPENSE, id)
        assertTrue(drafts.rows.isEmpty())
        assertEquals(1, commands.rows.size)
        val tombstone = commands.rows.values.single()
        assertTrue(tombstone.delete)
        remove(LedgerDataset.EXPENSE, id)
        assertEquals(1, commands.rows.size)
    }
}
