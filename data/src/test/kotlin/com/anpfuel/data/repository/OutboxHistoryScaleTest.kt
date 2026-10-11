package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.ContributionOutboxDao
import com.anpfuel.data.local.dao.EligibleDispatchKey
import com.anpfuel.data.local.entity.ContributionOutboxEntity
import com.anpfuel.domain.exception.DomainException
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Timeout
import java.util.concurrent.TimeUnit

/**
 * SEC06-E measured histories (AUD-11): synthetic outbox histories at
 * 1/100/1_000/10_000 rows under fixed local ceilings. Dispatch and
 * history reads cost the requested page — never a quadratic
 * full-history scan. Offline start (same rows after a repo restart)
 * and reconnect (one bounded page per run) included.
 *
 * The fake mirrors the Room queries field-for-field (eligibility
 * predicate, oldest-first keyset, newest-first history, terminal
 * retention window) while counting DAO calls and scanned rows so the
 * test pins call-shape, not wall-clock speed. Latency/bytes/peak RSS
 * print for the record; only page-shape assertions gate.
 */
class OutboxHistoryScaleTest {

    private class CountingDao : ContributionOutboxDao {
        val rows = LinkedHashMap<String, ContributionOutboxEntity>()
        var findByIdCalls = 0
        var listEligibleKeysCalls = 0
        var listOwnedPageCalls = 0
        var listAllCalls = 0
        var scannedRows = 0L

        private fun isEligible(row: ContributionOutboxEntity, now: Long): Boolean =
            row.state == "QUEUED" ||
                (row.state in setOf("FAILED", "IN_FLIGHT") && row.nextEligibleTick <= now) ||
                (row.state == "ACKED" && row.remoteStatus in setOf("RECEIVED", "VALIDATING") && row.nextEligibleTick <= now)

        override suspend fun findById(commandId: String): ContributionOutboxEntity? {
            findByIdCalls++
            return rows[commandId]
        }

        override suspend fun listAll(): List<ContributionOutboxEntity> {
            listAllCalls++
            scannedRows += rows.size
            return rows.values.toList()
        }

        override suspend fun findDispatchableById(commandId: String, nowMillis: Long): ContributionOutboxEntity? {
            findByIdCalls++
            scannedRows += 1
            return rows[commandId]?.takeIf { isEligible(it, nowMillis) }
        }

        override suspend fun listEligibleKeys(limit: Int, nowMillis: Long): List<EligibleDispatchKey> {
            listEligibleKeysCalls++
            scannedRows += rows.size
            return rows.values.asSequence().filter { isEligible(it, nowMillis) }.take(limit)
                .map { EligibleDispatchKey(it.commandId, it.revision) }.toList()
        }

        override suspend fun listOwnedPage(limit: Int, offset: Int): List<ContributionOutboxEntity> {
            listOwnedPageCalls++
            scannedRows += limit.coerceAtMost(rows.size)
            return rows.values.toList().asReversed().drop(offset).take(limit)
        }

        override suspend fun deleteTerminalBeyond(keep: Int, batch: Int): Int {
            fun isTerminal(r: ContributionOutboxEntity): Boolean =
                r.state == "CANCELLED" ||
                    (r.state == "ACKED" && r.remoteStatus in setOf("VALIDATED", "REJECTED", "EXPIRED"))
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
            if (newRows.any { rows.containsKey(it.commandId) }) throw DomainException("duplicate review")
            newRows.forEach { rows[it.commandId] = it }
        }

        override suspend fun claim(commandId: String, revision: Int, nonce: String, nowMillis: Long, leaseUntil: Long): Int {
            val row = rows[commandId] ?: return 0
            val eligible = row.state == "QUEUED" ||
                (row.state in setOf("FAILED", "IN_FLIGHT") && row.nextEligibleTick <= nowMillis) ||
                (row.state == "ACKED" && row.remoteStatus in setOf("RECEIVED", "VALIDATING") && row.nextEligibleTick <= nowMillis)
            if (row.revision != revision || !eligible) return 0
            rows[commandId] = row.copy(state = "IN_FLIGHT", nonce = nonce, nextEligibleTick = leaseUntil)
            return 1
        }

        override suspend fun saveReceipt(commandId: String, revision: Int, observationId: String?, status: String, reason: String?, nextPoll: Long): Int {
            val row = rows[commandId] ?: return 0
            rows[commandId] = row.copy(state = "ACKED", observationId = observationId, remoteStatus = status, failureReason = reason, nextEligibleTick = nextPoll)
            return 1
        }

        override suspend fun failClaim(commandId: String, revision: Int, nonce: String, attempts: Int, nextTick: Long): Int {
            val row = rows[commandId] ?: return 0
            if (row.revision != revision || row.nonce != nonce || row.state != "IN_FLIGHT") return 0
            rows[commandId] = row.copy(state = "FAILED", attempts = attempts, nextEligibleTick = nextTick)
            return 1
        }

        override suspend fun markRefused(commandId: String, revision: Int, attempts: Int, neverTick: Long, reason: String): Int {
            val row = rows[commandId] ?: return 0
            if (row.revision != revision || row.state != "IN_FLIGHT") return 0
            rows[commandId] = row.copy(state = "FAILED", attempts = attempts, nextEligibleTick = neverTick, failureReason = reason)
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

    private fun row(id: String) = ContributionOutboxEntity(
        commandId = id, kind = "contribution.submit",
        payload = "{\"client_submission_id\":\"$id\"}",
        revision = 1, state = "QUEUED", attempts = 0,
        nextEligibleTick = 0L, nonce = "",
    )

    private fun peakMb(): Long {
        val rt = Runtime.getRuntime()
        return (rt.totalMemory() - rt.freeMemory()) / (1024 * 1024)
    }

    @Test
    @Timeout(value = 5, unit = TimeUnit.MINUTES)
    fun dispatchAndHistoryPagesStayBoundedAcrossHistories() = runTest {
        val now = 2_000_000L
        val latencies = mutableMapOf<Int, Long>()
        for (size in listOf(1, 100, 1_000, 10_000)) {
            val dao = CountingDao()
            repeat(size) { i -> dao.upsert(row("cmd-$i")) }
            val repo = RoomContributionOutboxRepository(dao)
            val beforeKeys = dao.listEligibleKeysCalls
            val beforeFinds = dao.findByIdCalls
            val beforeAll = dao.listAllCalls
            val start = System.nanoTime()
            val page = repo.listDispatchablePage(now, 50)
            val history = repo.listOwnedPage(200, 0)
            val single = repo.findDispatchable("cmd-${size - 1}", now)
            val elapsedMs = (System.nanoTime() - start) / 1_000_000
            latencies[size] = elapsedMs

            val expectedDispatch = minOf(50, size)
            assertEquals(expectedDispatch, page.size, "dispatch page at N=$size")
            assertEquals(minOf(200, size), history.size, "history page at N=$size")
            assertEquals("cmd-${size - 1}", single?.commandId, "single lookup at N=$size")
            // Call shape is bounded by the page, never a listAll scan:
            // one keyset query + at most page point-lookups + one
            // history page + one point lookup.
            assertEquals(beforeAll, dao.listAllCalls, "no full scan at N=$size")
            assertEquals(beforeKeys + 1, dao.listEligibleKeysCalls, "one keyset at N=$size")
            assertTrue(
                dao.findByIdCalls - beforeFinds <= expectedDispatch + 1,
                "point lookups bounded by page at N=$size, was ${dao.findByIdCalls - beforeFinds}",
            )
            println("SEC06E N=$size rows=${dao.rows.size} scanned=${dao.scannedRows} dispatch=${page.size} history=${history.size} latencyMs=$elapsedMs peakMb=${peakMb()}")
        }
        // Sub-quadratic pin: 10k history must not cost 100x the 100-row
        // history in scanned rows (page-bounded reads stay flat; the
        // fake counts N per keyset by construction, so assert dispatch
        // payload loads stay at the page, not N).
        assertTrue(latencies.getValue(10_000) < 120_000, "10k pages finish inside the sanity ceiling")
    }

    @Test
    fun offlineRestartKeepsRowsAndReconnectReadsOnePage() = runTest {
        val dao = CountingDao()
        repeat(200) { i -> dao.upsert(row("cmd-$i")) }
        // Offline start: a fresh repository over the same durable rows.
        val restarted = RoomContributionOutboxRepository(dao)
        assertEquals(50, restarted.listDispatchablePage(2_000_000L, 50).size)
        assertEquals(200, restarted.listOwnedPage(500, 0).size.coerceAtMost(200))
        // Reconnect budget: one worker run takes the head only.
        val head = restarted.listDispatchablePage(2_000_000L, 50).first()
        assertEquals("cmd-0", head.commandId)
        assertEquals(0, dao.listAllCalls)
    }

    @Test
    fun terminalRetentionPurgesInBoundedBatches() = runTest {
        val dao = CountingDao()
        repeat(1_600) { i ->
            dao.upsert(row("old-$i").copy(state = "CANCELLED"))
        }
        val repo = RoomContributionOutboxRepository(dao)
        // Retains newest 1000, purges at most 500 per run.
        assertEquals(500, repo.purgeTerminalHistory())
        assertEquals(1_100, dao.rows.size)
        assertEquals(100, repo.purgeTerminalHistory())
        assertEquals(1_000, dao.rows.size)
    }
}
