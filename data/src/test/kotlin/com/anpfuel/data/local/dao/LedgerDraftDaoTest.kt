package com.anpfuel.data.local.dao

import com.anpfuel.data.local.entity.LedgerDraftEntity
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class LedgerDraftDaoTest {

    private class FakeDraftDao : LedgerDraftDao {
        val rows = mutableMapOf<String, LedgerDraftEntity>()

        override suspend fun upsert(entity: LedgerDraftEntity) {
            rows[entity.draftId] = entity
        }

        override suspend fun findByRecord(owner: String, origin: String, dataset: String, recordId: String) =
            rows.values.firstOrNull {
                it.ownerAccount == owner && it.origin == origin &&
                    it.dataset == dataset && it.recordId == recordId
            }

        override suspend fun listOwned(owner: String, origin: String) =
            rows.values.filter { it.ownerAccount == owner && it.origin == origin }
                .sortedBy { it.updatedAtMillis }

        override suspend fun deleteByRecord(owner: String, origin: String, dataset: String, recordId: String) {
            rows.entries.removeIf {
                it.value.ownerAccount == owner && it.value.origin == origin &&
                    it.value.dataset == dataset && it.value.recordId == recordId
            }
        }

        override suspend fun compareAndSetBody(owner: String, origin: String, dataset: String, recordId: String, expectedRevision: Long, expectedBody: String, replacementBody: String, nowMillis: Long, draftId: String): Int {
            val row = findByRecord(owner,origin,dataset,recordId) ?: return 0
            if (row.draftId != draftId || row.expectedRevision != expectedRevision || row.bodyJson != expectedBody) return 0
            rows[row.draftId] = row.copy(bodyJson = replacementBody, updatedAtMillis = nowMillis)
            return 1
        }

        override suspend fun clearOwner(owner: String, origin: String) {
            rows.entries.removeIf { it.value.ownerAccount == owner && it.value.origin == origin }
        }
    }

    private fun draft(
        draftId: String = "op-1",
        owner: String = "acct-1",
        origin: String = "https://api.example.test",
        dataset: String = "refuel",
        record: String = "rec-1",
        revision: Long = 0,
    ) = LedgerDraftEntity(
        draftId = draftId, ownerAccount = owner, origin = origin, dataset = dataset,
        recordId = record, expectedRevision = revision, bodyJson = """{"id":"$record"}""",
        updatedAtMillis = 1,
    )

    @Test fun `drafts save update delete within one partition`() = runTest {
        val dao = FakeDraftDao()
        dao.upsert(draft())
        assertEquals("rec-1", dao.findByRecord("acct-1", "https://api.example.test", "refuel", "rec-1")?.recordId)
        dao.upsert(draft(revision = 1))
        assertEquals(1L, dao.findByRecord("acct-1", "https://api.example.test", "refuel", "rec-1")?.expectedRevision)
        assertEquals(1, dao.listOwned("acct-1", "https://api.example.test").size)
        dao.deleteByRecord("acct-1", "https://api.example.test", "refuel", "rec-1")
        assertNull(dao.findByRecord("acct-1", "https://api.example.test", "refuel", "rec-1"))
    }

    @Test fun `partitions never leak across owners or origins`() = runTest {
        val dao = FakeDraftDao()
        dao.upsert(draft(owner = "acct-1"))
        dao.upsert(draft(draftId = "op-2", owner = "acct-2"))
        dao.upsert(draft(draftId = "op-3", origin = "https://other.example/"))
        assertEquals(1, dao.listOwned("acct-1", "https://api.example.test").size)
        dao.clearOwner("acct-1", "https://api.example.test")
        assertEquals(0, dao.listOwned("acct-1", "https://api.example.test").size)
        assertEquals(1, dao.listOwned("acct-2", "https://api.example.test").size)
        assertEquals(1, dao.listOwned("acct-1", "https://other.example/").size)
    }
}
