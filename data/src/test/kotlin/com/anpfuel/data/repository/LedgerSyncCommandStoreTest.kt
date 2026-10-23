package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.PremiumOutboxDao
import com.anpfuel.domain.premium.PremiumOwnerScope
import io.mockk.*
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class LedgerSyncCommandStoreTest {
    private val dao = mockk<PremiumOutboxDao>(relaxed = true)
    private val store = PremiumCommandStoreImpl(dao)
    private val owner = PremiumOwnerScope("a", "https://api.example.test")

    @Test fun `record cleanup is owner scoped and refuses active worker claims`() = runTest {
        coEvery { dao.countActiveRecord("a", owner.origin, "refuel", "r") } returns 1
        assertThrows(IllegalStateException::class.java) { runBlocking { store.removeRecord(owner, "refuel", "r") } }
        coVerify(exactly = 0) { dao.deleteRecord(any(), any(), any(), any()) }
        coEvery { dao.countActiveRecord("a", owner.origin, "refuel", "r") } returns 0
        store.removeRecord(owner, "refuel", "r")
        coVerify(exactly = 1) { dao.deleteRecord("a", owner.origin, "refuel", "r") }
    }

    @Test fun `status counts bind exact dataset and bound the reported count`() = runTest {
        coEvery { dao.countPending("a", owner.origin, "refuel") } returns 123
        coEvery { dao.countPending("a", owner.origin, "expense") } returns 2
        assertEquals(101, store.countPending(owner, 101, "refuel"))
        assertEquals(2, store.countPending(owner, 101, "expense"))
    }
}
