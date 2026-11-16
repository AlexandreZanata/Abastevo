package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.EnterpriseLocalIdentity
import com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.trip.EnterpriseMissionProducers
import com.anpfuel.data.enterprise.ProtectedQueueFixture.attempt
import com.anpfuel.data.enterprise.ProtectedQueueFixture.id
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.C
import com.anpfuel.data.enterprise.VaultFixture.Companion.ORIGIN
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.data.enterprise.trip.ProtectedMissionStore
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.trip.*
import java.nio.file.Files
import java.nio.file.Path
import java.time.Instant
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

/** Synthetic missions bound like the vault fixture's grant: membership B, enrollment C, employee 930. */
internal object MissionStoreFixture {
    val employee = id(930)
    val clock = EnterpriseDeviceClock("boot-3", 1_000, 1_763_200_000_000L)
    fun item(mission: Int, state: TripMissionState = TripMissionState.ASSIGNED, revision: Long = 3) = MissionInboxItem(id(mission), id(940), id(941),
        "SYNTHETIC-PRIVATE-MISSION", Instant.parse("2026-11-20T11:00:00Z"), Instant.parse("2026-11-20T15:00:00Z"), state, revision, employee,
        if (state == TripMissionState.ACCEPTED) TripMissionResponse.ACCEPT else null, null,
        MissionPlanView(2, "SYNTHETIC-PRIVATE-PLACE", null, "Entrega programada", null, TripMissionLoad.NONE, null, null,
            Instant.parse("2026-11-15T10:10:00Z")), Instant.parse("2026-11-15T10:00:00Z"), Instant.parse("2026-11-15T10:30:00Z"))
    fun page(vararg items: MissionInboxItem) = MissionInboxPage(items.toList(), false)
    fun respond(p: EnterprisePartition, op: Int, mission: Int, revision: Long = 3, response: String = "ACCEPT", reason: String? = null): EnterpriseOperation =
        TripMissionOperations.respond(TripEnvelope(SyncBinding.of(p.ownerAccount, p.origin, p.organizationId), B, C, id(op),
            TimeDeclaration(Instant.parse("2026-11-15T12:00:00Z"), "America/Cuiaba", -14_400)), id(mission), revision, response, reason)
    private val producer = EnterpriseMissionProducers.all().single()
    suspend fun save(queue: ProtectedEnterpriseQueue, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation): Boolean =
        queue.save(attempt, operation) { tx -> producer.save(tx, attempt, operation) }
}

class ProtectedMissionStoreTest {
    @TempDir lateinit var root: Path
    private val f = MissionStoreFixture

    @Test fun `a server page is kept without an operation and an answer commits with its operation in the exact partition`(): Unit = runBlocking {
        val v = VaultFixture(root); v.confirm(partition()); v.confirm(partition(org = B))
        val store = ProtectedMissionStore(v.store); val queue = ProtectedEnterpriseQueue(v.store)
        assertNull(store.read(attempt(v)).inbox)
        store.update(attempt(v)) { it.withPage(f.employee, f.page(f.item(950), f.item(951)), f.clock) }
        assertEquals(listOf(id(950), id(951)), store.read(attempt(v)).inbox!!.items.map { it.missionId })
        assertTrue(queue.pending(attempt(v)).isEmpty(), "a page is no operation")
        val answer = f.respond(partition(), 960, 950)
        assertTrue(f.save(queue, attempt(v), answer))
        assertFalse(f.save(queue, attempt(v), answer), "an identical retry is idempotent")
        val snapshot = store.read(attempt(v))
        assertEquals(listOf(id(960)), snapshot.inbox!!.responses.map { it.operationId })
        assertEquals(setOf(id(960)), snapshot.entries.keys)
        assertArrayEquals(answer.canonicalBytes, snapshot.entries.getValue(id(960)).operation.canonicalBytes)
        assertNull(store.read(attempt(v, partition(org = B))).inbox, "another organization of the same account reads nothing")
        // A refresh keeps the saved answer and its entry.
        store.update(attempt(v)) { it.withPage(f.employee, f.page(f.item(951)), f.clock.copy(elapsedMillis = 2_000)) }
        assertEquals(setOf(id(960)), store.read(attempt(v)).entries.keys)
        // A local refusal inside the protected write keeps its own failure and writes nothing.
        val before = store.read(attempt(v)).inbox!!.canonicalBytes
        val refusal = assertThrows(MissionLocalException::class.java) { runBlocking {
            store.update(attempt(v)) { it.withPage(id(931), f.page(f.item(952)), f.clock) } } }
        assertEquals(MissionLocalFailure.FOREIGN, refusal.failure)
        assertArrayEquals(before, store.read(attempt(v)).inbox!!.canonicalBytes)
        // Another account on the same device opens nothing of A.
        val stale = attempt(v)
        v.identity = EnterpriseLocalIdentity(C, ORIGIN, C); v.fence.lock()
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { store.read(stale) } }
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { store.update(stale) { it } } }
    }

    @Test fun `the producer refuses an answer the copy does not allow and nothing is written`(): Unit = runBlocking {
        val v = VaultFixture(root); v.confirm(partition())
        val store = ProtectedMissionStore(v.store); val queue = ProtectedEnterpriseQueue(v.store)
        store.update(attempt(v)) { it.withPage(f.employee, f.page(f.item(950), f.item(951, TripMissionState.ACCEPTED, 4)), f.clock) }
        val before = store.read(attempt(v)).inbox!!.canonicalBytes
        for ((op, answer) in listOf(970 to f.respond(partition(), 970, 959), 971 to f.respond(partition(), 971, 951, 4),
            972 to f.respond(partition(), 972, 950, 2))) {
            assertThrows(Exception::class.java) { runBlocking { f.save(queue, attempt(v), answer) } }
            assertNull(queue.entry(attempt(v), id(op)))
        }
        assertArrayEquals(before, store.read(attempt(v)).inbox!!.canonicalBytes)
    }

    @Test fun `a foreign or unreadable copy is never replaced and reads as unavailable`(): Unit = runBlocking {
        for ((index, stored) in listOf(MissionInbox.empty(B, id(9)).canonicalBytes, MissionInbox.empty(id(8), C).canonicalBytes,
            "{\"format\":\"mission-inbox\",\"version\":2}".toByteArray()).withIndex()) {
            val v = VaultFixture(root.resolve("vault-$index")); v.confirm(partition())
            v.store.put(EnterpriseLocalRecord(partition(), EnterpriseMissionProducers.INBOX_RECORD_ID, stored))
            val store = ProtectedMissionStore(v.store)
            assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { store.read(attempt(v)) } }
            assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { store.update(attempt(v)) { it.withPage(f.employee, f.page(f.item(950)), f.clock) } } }
            assertArrayEquals(stored, v.store.record(partition(), EnterpriseMissionProducers.INBOX_RECORD_ID)!!.envelope)
        }
    }

    @Test fun `the sealed generation holds no plaintext mission fact`(): Unit = runBlocking {
        val v = VaultFixture(root); v.confirm(partition())
        val store = ProtectedMissionStore(v.store); val queue = ProtectedEnterpriseQueue(v.store)
        store.update(attempt(v)) { it.withPage(f.employee, f.page(f.item(950)), f.clock) }
        f.save(queue, attempt(v), f.respond(partition(), 960, 950))
        Files.walk(root).use { files ->
            files.filter(Files::isRegularFile).forEach { file ->
                val text = String(Files.readAllBytes(file), Charsets.ISO_8859_1)
                assertFalse(text.contains("SYNTHETIC-PRIVATE") || text.contains("mission-inbox") || text.contains(id(950)) ||
                    text.contains("trip.mission.respond") || text.contains("ACCEPT"), file.toString())
            }
        }
    }
}
