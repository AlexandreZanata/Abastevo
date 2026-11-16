package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.EnterpriseAccessResult
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.EnterpriseLocalIdentity
import com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.queue.EnterpriseQueueSource
import com.anpfuel.application.enterprise.tracking.EnterpriseTrackingProducers
import com.anpfuel.application.enterprise.tracking.EnterpriseTrackingService
import com.anpfuel.application.enterprise.trip.EnterpriseTripProducers
import com.anpfuel.data.enterprise.ProtectedQueueFixture.attempt
import com.anpfuel.data.enterprise.ProtectedQueueFixture.id
import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.C
import com.anpfuel.data.enterprise.VaultFixture.Companion.ORIGIN
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.data.enterprise.tracking.ProtectedTrackingStore
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.tracking.LocationPermission
import com.anpfuel.domain.enterprise.tracking.TrackingCause
import com.anpfuel.domain.enterprise.tracking.TrackingCheckpoint
import com.anpfuel.domain.enterprise.tracking.TrackingDevice
import com.anpfuel.domain.enterprise.tracking.TrackingPolicy
import com.anpfuel.domain.enterprise.tracking.TrackingPolicyRead
import com.anpfuel.domain.enterprise.tracking.TrackingSessionState
import java.nio.file.Path
import java.time.Instant
import java.time.ZoneId
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

/** EMP-GPS-002 sealed tracking storage over the real protected generation (EDR-322/324): binding, isolation and restart. */
class ProtectedTrackingStoreTest {
    @TempDir lateinit var root: Path

    private val trackingProducers = EnterpriseTrackingProducers.all().associateBy { it.descriptor().type }
    private val tripProducers = EnterpriseTripProducers.all(TripStoreFixture.hints).associateBy { it.descriptor().type }
    private fun producer(operation: EnterpriseOperation) = trackingProducers[operation.type] ?: tripProducers.getValue(operation.type)

    /** The real queue save, the operation and its producer's projection in one protected write, over the vault fixture. */
    private inner class Device(val f: VaultFixture, var wall: Long = Instant.parse("2026-11-15T12:30:00Z").toEpochMilli(), firstId: Int = 3000) {
        val queue = ProtectedEnterpriseQueue(f.store)
        val store = ProtectedTrackingStore(f.store)
        // Each process draws new random identifiers in production; distinct synthetic ranges stand for that here.
        private var nonce = firstId
        val source = object : EnterpriseQueueSource {
            override suspend fun account(): EnterpriseQueueAccountLease = ProtectedQueueFixture.account(f)
            override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt =
                account.forEnrollment(f.store.protectedEnrollment(account, partition))
        }
        val context = EnterpriseContextSummary(partition(), B, "Empresa sintética", id(630))
        val policy = TrackingPolicyRead(TrackingPolicy.EXPECTED, 2, "aviso-2026.1", Instant.parse("2026-11-01T00:00:00Z"), Instant.ofEpochMilli(wall))
        val service = EnterpriseTrackingService(source, { p, operation ->
            val captured = source.attempt(source.account(), p)
            queue.save(captured, operation) { tx -> producer(operation).save(tx, captured, operation) }
        }, store, { _, at -> EnterpriseAccessResult.Ok(TrackingPolicyRead(policy.policy, policy.revision, policy.noticeVersion, policy.noticeInForceAt, at)) },
            { ZoneId.of("America/Cuiaba") }, { wall }, { id(nonce++) })
        suspend fun startTrip() {
            val captured = attempt(f)
            assertTrue(queue.save(captured, TripStoreFixture.start()) { tx -> producer(TripStoreFixture.start()).save(tx, captured, TripStoreFixture.start()) })
        }
    }

    @Test fun `the book, the policy and the checkpoint commit in the exact partition and nothing else reads them`(): Unit = runBlocking {
        val f = VaultFixture(root); f.confirm(partition()); f.confirm(partition(org = B))
        val d = Device(f)
        assertNull(d.store.read(attempt(f)).book)
        d.startTrip()
        d.service.refreshPolicy(d.source.account(), d.context)
        val view = d.service.start(d.source.account(), d.context, id(610), TrackingDevice(LocationPermission.PRECISE, true))
        assertEquals(TrackingSessionState.RECORDING, view.state)
        val snapshot = d.store.read(attempt(f))
        val session = snapshot.book!!.session(id(610))!!
        assertEquals(TrackingPolicy.EXPECTED, snapshot.book!!.policy!!.policy)
        assertEquals(setOf(id(600)) + session.operationIds, snapshot.entries.keys, "the trip start and the session start, read in one snapshot")
        assertTrue(d.store.checkpoint(attempt(f), session.sessionId, TrackingCheckpoint("boot-3", Instant.ofEpochMilli(d.wall))))
        assertFalse(d.store.checkpoint(attempt(f), id(1), TrackingCheckpoint("boot-3", Instant.ofEpochMilli(d.wall))), "never creates a session")
        assertEquals(TrackingCheckpoint("boot-3", Instant.ofEpochMilli(d.wall)), d.store.read(attempt(f)).book!!.sessionById(session.sessionId)!!.checkpoint)
        assertNull(d.store.read(attempt(f, partition(org = B))).book, "another organization of the same account reads nothing")
        // Another account on the same device opens nothing of A.
        val stale = attempt(f)
        f.identity = EnterpriseLocalIdentity(C, ORIGIN, C); f.fence.lock()
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { d.store.read(stale) } }
    }

    @Test fun `a foreign or unreadable book is unavailable and never replaced`(): Unit = runBlocking {
        val f = VaultFixture(root); f.confirm(partition())
        f.store.put(EnterpriseLocalRecord(partition(), EnterpriseTrackingProducers.BOOK_RECORD_ID, "not a book".toByteArray()))
        val store = ProtectedTrackingStore(f.store)
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { store.read(attempt(f)) } }
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking {
            store.replacePolicy(attempt(f), TrackingPolicyRead(TrackingPolicy.OFF, 0, null, null, Instant.parse("2026-11-15T12:00:00Z"))) } }
        assertArrayEquals("not a book".toByteArray(), f.store.record(partition(), EnterpriseTrackingProducers.BOOK_RECORD_ID)!!.envelope)
    }

    @Test fun `after a restart the recording session is interrupted at its checkpoint and only the driver resumes it`(): Unit = runBlocking {
        val vault = root.resolve("vault"); val provider = root.resolve("provider")
        val first = VaultFixture(vault, FileTestVaultKeys(provider)); first.confirm(partition())
        val before = Device(first)
        before.startTrip()
        before.service.refreshPolicy(before.source.account(), before.context)
        val session = before.service.start(before.source.account(), before.context, id(610), TrackingDevice(LocationPermission.PRECISE, true)).sessionId
        before.wall += 60_000
        assertTrue(before.service.checkpoint(before.source.account(), partition(), session, "boot-3"))
        val checkpointAt = Instant.ofEpochMilli(before.wall)
        // The process dies: a new process opens the same protected generation with no collector running.
        val reopened = VaultFixture(vault, FileTestVaultKeys(provider))
        val after = Device(reopened, before.wall + 15 * 60_000, firstId = 4000)
        val restored = after.store.read(attempt(reopened)).book!!.sessionById(session)!!
        assertEquals(TrackingSessionState.RECORDING, restored.state, "the book survives the restart")
        assertEquals(checkpointAt, restored.checkpoint!!.at)
        assertEquals(1, after.service.recover(after.source.account(), partition(), { false }, "boot-4"))
        val interrupted = after.service.screen(after.source.account(), partition()).session(id(610))!!
        assertEquals(TrackingSessionState.INTERRUPTED, interrupted.state)
        assertEquals(TrackingCause.DEVICE_RESTARTED, interrupted.cause)
        assertEquals(checkpointAt, interrupted.since)
        assertEquals(0, after.service.recover(after.source.account(), partition(), { false }, "boot-4"), "nothing resumes by itself")
        assertNull(after.service.collecting(after.source.account(), partition(), session))
        val resumed = after.service.start(after.source.account(), after.context, id(610), TrackingDevice(LocationPermission.PRECISE, true))
        assertEquals(TrackingSessionState.RECORDING, resumed.state)
        assertEquals(3, resumed.records.size)
        // Every session operation is sealed to the partition's membership and enrollment (BR-OFFL-003).
        val entries = after.store.read(attempt(reopened)).entries
        assertTrue(resumed.records.all { entries.getValue(it.operationId).operation.let { op -> op.membershipId == B && op.enrollmentId == C } })
    }
}
