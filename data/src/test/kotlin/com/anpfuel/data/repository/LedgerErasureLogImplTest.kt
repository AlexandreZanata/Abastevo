package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.LedgerErasedDao
import com.anpfuel.data.local.entity.LedgerErasedEntity
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class LedgerErasureLogImplTest {
    private class Dao : LedgerErasedDao {
        val rows = mutableMapOf<List<String>, LedgerErasedEntity>()
        override suspend fun record(erasure: LedgerErasedEntity): Long {
            val key = listOf(erasure.ownerAccount, erasure.origin, erasure.dataset, erasure.recordId)
            return if (rows.putIfAbsent(key, erasure) == null) 1L else -1L
        }
        override suspend fun isErased(owner: String, origin: String, dataset: String, recordId: String) = rows.containsKey(listOf(owner, origin, dataset, recordId))
        override suspend fun clearOwner(owner: String, origin: String) { rows.keys.removeAll { it[0] == owner && it[1] == origin } }
    }
    @Test fun `idempotent insert retains first epoch and partitions owner and origin`() = runTest {
        val dao = Dao()
        val log = LedgerErasureLogImpl(dao)
        log.recordErased("a", "origin", "refuel", "r", 1)
        log.recordErased("a", "origin", "refuel", "r", 2)
        log.recordErased("b", "origin", "refuel", "r", 3)
        log.recordErased("a", "other", "refuel", "r", 4)
        assertEquals(1L, dao.rows[listOf("a", "origin", "refuel", "r")]!!.erasedAtMillis)
        assertTrue(log.isErased("a", "origin", "refuel", "r"))
        assertFalse(log.isErased("a", "origin", "expense", "r"))
        log.clearOwner("a", "origin")
        assertFalse(log.isErased("a", "origin", "refuel", "r"))
        assertTrue(log.isErased("b", "origin", "refuel", "r"))
        assertTrue(log.isErased("a", "other", "refuel", "r"))
    }
    @Test fun `blank owner performs no writes or reads`() = runTest {
        val dao = Dao()
        val log = LedgerErasureLogImpl(dao)
        log.recordErased("", "o", "refuel", "r", 1)
        log.clearOwner("", "o")
        assertFalse(log.isErased("", "o", "refuel", "r"))
        assertTrue(dao.rows.isEmpty())
    }
}
