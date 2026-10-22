package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.LedgerDraftDao
import com.anpfuel.data.local.entity.LedgerDraftEntity
import com.anpfuel.domain.premium.ExpenseCategory
import com.anpfuel.domain.premium.ExpenseDraft
import com.anpfuel.domain.premium.LedgerDataset
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import java.time.Instant
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class LedgerDraftStoreImplTest {

    private val dao = mockk<LedgerDraftDao>()
    private val store = LedgerDraftStoreImpl(dao)
    private val draft = ExpenseDraft(
        "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222",
        Instant.parse("2026-03-15T10:30:00Z"), "UTC", 5000, ExpenseCategory.TOLLS, null, null,
    )

    @Test fun `save maps fields and decodes back`() = runTest {
        val captured = slot<LedgerDraftEntity>()
        coEvery { dao.upsert(capture(captured)) } returns Unit
        val stored = store.save("acct-1", "https://api.example.test", draft, 0, 7L)
        assertEquals(draft, stored.draft)
        assertEquals(0L, stored.expectedRevision)
        assertEquals(7L, stored.updatedAtMillis)
        assertEquals("expense", captured.captured.dataset)
        coEvery { dao.findByRecord("acct-1", "https://api.example.test", "expense", draft.recordId) } returns captured.captured
        assertEquals(draft, store.find("acct-1", "https://api.example.test", LedgerDataset.EXPENSE, draft.recordId)?.draft)
        coEvery { dao.findByRecord(any(), any(), any(), any()) } returns null
        assertNull(store.find("acct-2", "https://api.example.test", LedgerDataset.EXPENSE, draft.recordId))
        coVerify(exactly = 1) { dao.upsert(any()) }
    }

    @Test fun `corrupt stored bodies fail closed`() = runTest {
        coEvery { dao.findByRecord(any(), any(), any(), any()) } returns LedgerDraftEntity(
            "d", "acct-1", "https://api.example.test", "expense", draft.recordId, 0, """{"id":"nope"}""", 1,
        )
        try {
            store.find("acct-1", "https://api.example.test", LedgerDataset.EXPENSE, draft.recordId)
            error("corrupt draft decoded")
        } catch (e: com.anpfuel.domain.premium.PremiumValidationException) {
            assertEquals("corrupt-draft", e.code)
        }
    }
}
