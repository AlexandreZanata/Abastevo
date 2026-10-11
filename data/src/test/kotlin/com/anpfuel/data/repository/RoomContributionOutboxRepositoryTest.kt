package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.ContributionOutboxDao
import com.anpfuel.data.local.dao.EligibleDispatchKey
import com.anpfuel.data.local.entity.ContributionOutboxEntity
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.ContributionDraft
import com.anpfuel.domain.repository.OwnedContribution
import com.anpfuel.domain.repository.OwnedContributionPhase
import com.anpfuel.domain.repository.PendingContribution
import com.anpfuel.domain.valueobject.FuelProduct
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
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
        var listAllCalls = 0

        override suspend fun findById(commandId: String): ContributionOutboxEntity? =
            rows[commandId]

        override suspend fun listAll(): List<ContributionOutboxEntity> {
            listAllCalls++
            return rows.values.toList()
        }

        private fun isEligible(row: ContributionOutboxEntity, nowMillis: Long): Boolean =
            row.state == "QUEUED" || (row.state in setOf("FAILED", "IN_FLIGHT") && row.nextEligibleTick <= nowMillis) ||
                (row.state == "ACKED" && row.remoteStatus in setOf("RECEIVED", "VALIDATING") && row.nextEligibleTick <= nowMillis)

        override suspend fun findDispatchableById(commandId: String, nowMillis: Long): ContributionOutboxEntity? =
            rows[commandId]?.takeIf { isEligible(it, nowMillis) }

        override suspend fun listEligibleKeys(limit: Int, nowMillis: Long): List<EligibleDispatchKey> =
            rows.values.filter { isEligible(it, nowMillis) }.take(limit)
                .map { EligibleDispatchKey(it.commandId, it.revision) }

        override suspend fun listOwnedPage(limit: Int, offset: Int): List<ContributionOutboxEntity> =
            rows.values.toList().asReversed().drop(offset).take(limit)

        override suspend fun deleteTerminalBeyond(keep: Int, batch: Int): Int {
            fun isTerminal(row: ContributionOutboxEntity): Boolean =
                row.state == "CANCELLED" || (row.state == "ACKED" && row.remoteStatus in setOf("VALIDATED", "REJECTED", "EXPIRED"))
            val ordered = rows.values.toList()
            val retained = ordered.takeLast(keep.coerceAtLeast(0)).map { it.commandId }.toSet()
            val victims = ordered.filter { isTerminal(it) && it.commandId !in retained }.take(batch)
            victims.forEach { rows.remove(it.commandId) }
            return victims.size
        }

        override suspend fun upsert(entity: ContributionOutboxEntity) {
            rows[entity.commandId] = entity
        }

        override suspend fun insertReview(newRows: List<ContributionOutboxEntity>) {
            if (newRows.any { rows.containsKey(it.commandId) } || newRows.map { it.commandId }.distinct().size != newRows.size) throw DomainException("duplicate review")
            newRows.forEach { rows[it.commandId] = it }
        }

        override suspend fun claim(commandId: String, revision: Int, nonce: String, nowMillis: Long, leaseUntil: Long): Int {
            val row = rows[commandId] ?: return 0
            val eligible = row.state == "QUEUED" || (row.state in setOf("FAILED", "IN_FLIGHT") && row.nextEligibleTick <= nowMillis) ||
                (row.state == "ACKED" && row.remoteStatus in setOf("RECEIVED", "VALIDATING") && row.nextEligibleTick <= nowMillis)
            if (row.revision != revision || !eligible) return 0
            rows[commandId] = row.copy(state = "IN_FLIGHT", nonce = nonce, nextEligibleTick = leaseUntil)
            return 1
        }

        override suspend fun saveReceipt(commandId: String, revision: Int, observationId: String?, status: String, reason: String?, nextPoll: Long): Int {
            val row = rows[commandId] ?: return 0
            if (row.state == "CANCELLED" || (row.observationId != null && row.observationId != observationId) || row.revision != revision || (row.remoteStatus != null && row.remoteStatus !in setOf("RECEIVED", "VALIDATING", status))) return 0
            rows[commandId] = row.copy(state = "ACKED", observationId = observationId, remoteStatus = status, failureReason = reason, nextEligibleTick = nextPoll)
            return 1
        }

        override suspend fun failClaim(commandId: String, revision: Int, nonce: String, attempts: Int, nextTick: Long): Int {
            val row = rows[commandId] ?: return 0
            if (row.revision != revision || row.nonce != nonce || row.state != "IN_FLIGHT") return 0
            rows[commandId] = row.copy(state = "FAILED", attempts = attempts, nextEligibleTick = nextTick)
            return 1
        }

        override suspend fun cancelPending(commandId: String): Int {
            val row = rows[commandId] ?: return 0
            if (row.state !in setOf("QUEUED", "FAILED") || row.remoteStatus != null) return 0
            rows[commandId] = row.copy(state = "CANCELLED")
            return 1
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
        assertEquals(1, repo.listDispatchablePage(2_000_000L, 50).size)
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

        assertTrue(repo.listDispatchablePage(1_000_000L, 50).isEmpty())
        assertEquals(1, repo.listDispatchablePage(1_000_000L + 1_000L, 50).size)
    }

    @Test
    fun `owner listing carries phase revision and attempts`() = runTest {
        val dao = FakeDao()
        val repo = RoomContributionOutboxRepository(dao)

        repo.enqueue(draft("cmd-1"), payload())
        repo.markFailed("cmd-1", 1_000_000L)
        repo.enqueue(draft("cmd-2"), payload())

        val owned = repo.listOwnedPage(100, 0).sortedBy { it.commandId }
        assertEquals(2, owned.size)
        assertEquals(OwnedContributionPhase.FAILED, owned[0].phase)
        assertEquals(1, owned[0].revision)
        assertEquals(OwnedContributionPhase.QUEUED, owned[1].phase)
    }

    @Test
    fun `owner listing fails closed on corrupt state`() = runTest {
        val dao = FakeDao()
        dao.upsert(
            ContributionOutboxEntity(
                commandId = "cmd-9",
                kind = "contribution.submit",
                payload = payload(),
                revision = 1,
                state = "ACKED",
                attempts = 1,
                nextEligibleTick = 0L,
                nonce = "",
            ),
        )
        val repo = RoomContributionOutboxRepository(dao)

        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking { repo.listOwnedPage(100, 0) }
        }
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
    @Test
    fun `frozen review retry keeps ids and revisions and changed batch writes nothing`() = runTest {
        val dao = FakeDao()
        val repo = RoomContributionOutboxRepository(dao)
        val rows = listOf(PendingContribution(draft("row-1"), "one"), PendingContribution(draft("row-2"), "two"))
        val first = repo.enqueueReview(rows)
        assertEquals(first, repo.enqueueReview(rows))
        assertEquals(2, dao.rows.size)
        val changed = listOf(PendingContribution(draft("row-3"), "three"), rows[1].copy(payloadJson = "different"))
        try { repo.enqueueReview(changed); throw AssertionError("changed frozen review accepted") }
        catch (_: DomainException) { }
        assertEquals(setOf("row-1", "row-2"), dao.rows.keys)
    }

    // SEC04-D: owner purge deletes exactly the scoped rows and keeps
    // unscoped/unparseable rows (fail-closed) plus other owners.
    @Test
    fun `purgeOwned deletes only the erased owner`() = runTest {
        val dao = FakeDao()
        val repo = RoomContributionOutboxRepository(dao)
        fun row(id: String, scope: String?) = ContributionOutboxEntity(
            commandId = id, kind = "SUBMIT", state = "QUEUED", attempts = 0,
            nextEligibleTick = 0L, nonce = "",
            payload = if (scope == null) "{\"client_submission_id\":\"$id\"}"
            else "{\"client_submission_id\":\"$id\",\"scope\":{\"owner_scope\":\"$scope\",\"origin\":\"https://teste.abastevo.com.br\"}}",
            revision = 1,
        )
        dao.upsert(row("cmd-a1", "scope-a"))
        dao.upsert(row("cmd-a2", "scope-a"))
        dao.upsert(row("cmd-b1", "scope-b"))
        dao.upsert(row("cmd-legacy", null))
        dao.upsert(row("cmd-broken", "scope-a").copy(payload = "not json"))

        val purged = repo.purgeOwned("scope-a")

        assertEquals(2, purged)
        val remaining = dao.listAll().map { it.commandId }.toSet()
        assertEquals(setOf("cmd-b1", "cmd-legacy", "cmd-broken"), remaining)
        assertEquals(0, repo.purgeOwned(""))
        assertEquals(0, repo.purgeOwned("scope-unknown"))
    }

    // SEC06-C: dispatch pages cap at the limit with identifiers only;
    // single-command lookup never scans the table.
    @Test
    fun `dispatch selection stays bounded without full scans`() = runTest {
        val dao = FakeDao()
        val repo = RoomContributionOutboxRepository(dao)
        repeat(60) { i -> repo.enqueue(draft("cmd-$i"), payload()) }
        val callsBefore = dao.listAllCalls
        val page = repo.listDispatchablePage(2_000_000L, 50)
        assertEquals(50, page.size)
        assertEquals("cmd-0", page.first().commandId)
        val single = repo.findDispatchable("cmd-59", 2_000_000L)
        assertEquals("cmd-59", single?.commandId)
        assertEquals(null, repo.findDispatchable("cmd-missing", 2_000_000L))
        assertEquals(callsBefore, dao.listAllCalls)
    }

    // SEC06-C: history pages newest-first and bounded.
    @Test
    fun `history pages newest first bounded`() = runTest {
        val dao = FakeDao()
        val repo = RoomContributionOutboxRepository(dao)
        repeat(5) { i -> repo.enqueue(draft("cmd-$i"), payload()) }
        val first = repo.listOwnedPage(2, 0)
        assertEquals(listOf("cmd-4", "cmd-3"), first.map { it.commandId })
        val second = repo.listOwnedPage(2, 2)
        assertEquals(listOf("cmd-2", "cmd-1"), second.map { it.commandId })
        assertEquals("cmd-1", repo.findOwned("cmd-1")?.commandId)
        assertEquals(null, repo.findOwned("cmd-missing"))
        assertEquals(emptyList<OwnedContribution>(), repo.listOwnedPage(0, 0))
    }

    // SEC06-C retention: terminal history beyond the retained window
    // purges oldest-first in bounded batches; live rows never match.
    @Test
    fun `retention purges terminal beyond window bounded`() = runTest {
        val dao = FakeDao()
        val repo = RoomContributionOutboxRepository(dao)
        dao.upsert(
            ContributionOutboxEntity(
                commandId = "live-1", kind = "SUBMIT", payload = payload(),
                revision = 1, state = "QUEUED", attempts = 0,
                nextEligibleTick = 0L, nonce = "",
            ),
        )
        repeat(1005) { i ->
            dao.upsert(
                ContributionOutboxEntity(
                    commandId = "old-$i", kind = "SUBMIT", payload = payload(),
                    revision = 1, state = "CANCELLED", attempts = 0,
                    nextEligibleTick = 0L, nonce = "",
                ),
            )
        }
        assertEquals(5, repo.purgeTerminalHistory())
        assertEquals(1001, dao.rows.size)
        assertTrue(dao.rows.containsKey("live-1"))
        assertEquals(0, repo.purgeTerminalHistory())
        // Amortized on the write path: enquing converges retention.
        repo.enqueue(draft("cmd-new"), payload())
        assertTrue(dao.rows.size <= 1002)
    }
}
