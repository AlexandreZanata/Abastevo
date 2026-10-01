package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.ContributionOutboxDao
import com.anpfuel.data.local.entity.ContributionOutboxEntity
import com.anpfuel.domain.model.ContributionDraft
import com.anpfuel.domain.valueobject.FuelProduct
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P10-T05: process death survives in Room, duplicate send keeps one
 * command with bumped revision, stale acks never delete newer work.
 */
class RoomContributionOutboxRepositoryTest {

    private val stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"

    private class FakeDao : ContributionOutboxDao {
        val rows = mutableMapOf<String, ContributionOutboxEntity>()

        override suspend fun findById(commandId: String): ContributionOutboxEntity? =
            rows[commandId]

        override suspend fun listAll(): List<ContributionOutboxEntity> =
            rows.values.toList()

        override suspend fun upsert(entity: ContributionOutboxEntity) {
            rows[entity.commandId] = entity
        }

        override suspend fun deleteById(commandId: String) {
            rows.remove(commandId)
        }

        override suspend fun clear() {
            rows.clear()
        }
    }

    private fun draft(commandId: String = "cmd-1") = ContributionDraft.create(
        clientSubmissionId = commandId,
        stationId = stationId,
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        amountMilliBrl = 5890L,
        currency = "BRL",
        unit = "BRL/L",
        conditionKind = "STANDARD",
        capturedAtMillis = 1_000_000L,
    )

    private fun payload(freshness: String = "fresh") =
        "{\"client_submission_id\":\"cmd-1\",\"freshness\":\"$freshness\"}"

    @Test
    fun `enqueue then duplicate bumps revision keeping attempts`() = runTest {
        val repo = RoomContributionOutboxRepository(FakeDao())

        val first = repo.enqueue(draft(), payload("fresh"))
        val second = repo.enqueue(draft(), payload("historical"))

        assertEquals(1, first.revision)
        assertEquals(2, second.revision)
        assertEquals(1, repo.listDispatchable(2_000_000L).size)
        assertTrue(second.payloadJson.contains("historical"))
    }

    @Test
    fun `stale ack never deletes newer revision`() = runTest {
        val dao = FakeDao()
        val repo = RoomContributionOutboxRepository(dao)

        repo.enqueue(draft(), payload())
        repo.enqueue(draft(), payload())
        repo.markAcknowledged("cmd-1", 1)

        assertEquals(1, dao.rows.size)
        repo.markAcknowledged("cmd-1", 2)
        assertNull(dao.rows["cmd-1"])
    }

    @Test
    fun `failed command waits backoff then dispatches again`() = runTest {
        val repo = RoomContributionOutboxRepository(FakeDao())

        repo.enqueue(draft(), payload())
        repo.markFailed("cmd-1", 1_000_000L)

        assertTrue(repo.listDispatchable(1_000_000L).isEmpty())
        assertEquals(1, repo.listDispatchable(1_000_000L + 1_000L).size)
    }

    @Test
    fun `dispatched nonce is stored and old photo label survives`() = runTest {
        val dao = FakeDao()
        val repo = RoomContributionOutboxRepository(dao)

        repo.enqueue(draft(), payload("historical"))
        repo.markDispatched("cmd-1", "nonce-1")

        assertEquals("nonce-1", dao.rows.getValue("cmd-1").nonce)
        assertTrue(dao.rows.getValue("cmd-1").payload.contains("historical"))
    }
}
