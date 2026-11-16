package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.attachment.EnterpriseAttachmentDeliveryResult
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.C
import com.anpfuel.data.enterprise.VaultFixture.Companion.ORIGIN
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.sync.*
import java.nio.file.Path
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

/**
 * EMP-OFFL-011 (B-BR-EMP-536..539, BUC-EMP-275/276, EDR-348..350) on the real sealed store, queue, service and
 * worker with a scripted signed contact and transport. Synthetic identifiers and payloads only.
 */
class EnterpriseRevocationCleanupTest {
    @TempDir lateinit var root: Path

    private val pA = partition(org = A)
    private val pB = partition(org = B)
    private val foreign = partition(owner = C, org = A)
    private fun id(n: Int) = ProtectedQueueFixture.id(n)
    private fun token(p: EnterprisePartition) = EnterpriseVaultCodec.token(p)
    private fun exact(p: EnterprisePartition) = EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(p, B), C, 1)

    private fun ack(operation: EnterpriseOperation, result: String, code: String?): OperationAcknowledgement {
        val final = result != "RETRY"
        val review = if (result == "ACCEPTED_FOR_REVIEW") "\"${id(600)}\"" else "null"
        return OperationAcknowledgement.parse("""{"schema_version":1,"operation_id":"${operation.operationId}","payload_digest":"${operation.payloadDigest}","result":"$result","code":${code?.let { "\"$it\"" } ?: "null"},"entity_revision":${if (result == "ACCEPTED") "1" else "null"},"received_at":${if (final) "\"2026-11-16T12:00:01Z\"" else "null"},"review_item_id":$review,"warnings":[]}""".toByteArray(), operation)
    }

    private inner class Cleanup(val f: VaultFixture = VaultFixture(root.resolve("vault"))) {
        var verdict: (EnterpriseContactRequest) -> EnterpriseContactReply = { error("unexpected contact") }
        val contacts = mutableListOf<EnterpriseContactRequest>()
        val store = ProtectedEnterprisePersistence(f.files, f.keys, f.fence, f.erasure, { f.identity }, { f.now },
            EnterpriseContactAuthority { request -> contacts += request; verdict(request) })
        val queue = ProtectedEnterpriseQueue(store)
        val sent = mutableMapOf<String, MutableList<String>>()
        var answer: (EnterprisePartition, List<EnterpriseOperation>) -> EnterpriseOperationDelivery = { _, ops ->
            EnterpriseOperationDelivery(ops.map { ack(it, "ACCEPTED", null) })
        }
        val reviews = mutableListOf<String>()
        val uploads = mutableListOf<String>()
        val source = object : EnterpriseQueueSource {
            override suspend fun account(): EnterpriseQueueAccountLease = ProtectedQueueFixture.account(f)
            override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt =
                account.forEnrollment(store.protectedEnrollment(account, partition))
        }
        private val producer = object : EnterpriseQueueProducer {
            override fun descriptor() = EnterpriseQueueProducerDescriptor("validation.record")
            override fun save(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation) = Unit
            override fun acknowledge(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
                ack: OperationAcknowledgement) = Unit
        }
        val service = EnterpriseQueueService(source, queue, EnterpriseOperationTransport { attempt, operations ->
            sent.getOrPut(attempt.partition.organizationId + "/" + attempt.partition.ownerAccount) { mutableListOf() } += operations.map { it.operationId }
            answer(attempt.partition, operations)
        }, EnterpriseQueueRegistry(producer), { f.now }, { 0L })
        val runner = EnterpriseOperationWorkRunner(source, store::discover, { account, p -> service.dispatch(account, p) },
            { _, p -> reviews += p.organizationId; EnterpriseReviewPollResult() },
            { _, p -> uploads += p.organizationId; EnterpriseAttachmentDeliveryResult() },
            { account, p, hinted -> store.revocationCleanup(account, p, hinted) })

        fun sentFor(p: EnterprisePartition): List<String> = sent[p.organizationId + "/" + p.ownerAccount].orEmpty()
        suspend fun save(p: EnterprisePartition, n: Int): EnterpriseOperation =
            ProtectedQueueFixture.operation(p, id(n)).also { assertTrue(service.save(p, it)) }
        fun later(hours: Long = 7) {
            val step = hours * 3_600_000
            f.now = f.now.copy(elapsedMillis = f.now.elapsedMillis + step, wallMillis = f.now.wallMillis + step)
        }
        fun ended(p: EnterprisePartition) = f.keys.values.containsKey(EnterpriseVaultGenerationNames.ended(token(p)))
        fun material(p: EnterprisePartition) = f.keys.values.keys.filter { it.contains(token(p)) && !it.contains("_denied_") }
        fun foreignRecord(): ByteArray? {
            val saved = f.identity
            f.identity = EnterpriseLocalIdentity(C, ORIGIN, C)
            try { return runBlocking { f.store.record(foreign, A) }?.envelope } finally { f.identity = saved }
        }
        suspend fun seedForeignAccount() {
            val saved = f.identity
            f.identity = EnterpriseLocalIdentity(C, ORIGIN, C)
            try { f.confirm(foreign); f.store.put(EnterpriseLocalRecord(foreign, A, byteArrayOf(3))) } finally { f.identity = saved }
        }
        suspend fun runUntilQuiet(limit: Int = 4): List<EnterpriseOperationWorkResult> {
            val results = mutableListOf<EnterpriseOperationWorkResult>()
            do { results += runner.runOnce() } while (results.last().next == EnterpriseOperationWorkNext.Continue(0) && results.size < limit)
            return results
        }
    }

    /** BUC-EMP-275; negatives "foreign partition erased" and "eligible intake discarded silently". */
    @Test fun accessEndedDeliversEligibleIntakeThenErasesOnlyItsOwnPartition(): Unit = runBlocking {
        val c = Cleanup(); val f = c.f
        f.confirm(pA); f.confirm(pB); c.seedForeignAccount()
        f.store.put(EnterpriseLocalRecord(pA, A, byteArrayOf(1))); f.store.put(EnterpriseLocalRecord(pB, A, byteArrayOf(2)))
        val held = c.save(pA, 101); val unavailable = c.save(pA, 102); val other = c.save(pB, 201)
        c.verdict = { request ->
            assertEquals(exact(pA), request.existingEnrollment, "Contact must name the partition's retained exact enrollment")
            assertEquals(EnterpriseAccessBinding(A, ORIGIN, A, request.binding.epoch), request.binding)
            EnterpriseContactReply.Lock(request.existingEnrollment!!, A, 10_500)
        }
        c.answer = { p, ops -> EnterpriseOperationDelivery(ops.map {
            when {
                p != pA -> ack(it, "ACCEPTED", null)
                it.operationId == held.operationId -> ack(it, "ACCEPTED_FOR_REVIEW", "ACCESS_ENDED_BEFORE_RECEIPT")
                else -> ack(it, "RETRY", "UNAVAILABLE")
            }
        }) }
        val first = c.runUntilQuiet()
        assertEquals(1, c.contacts.size, "One exact contact after the late-intake hint")
        assertTrue(c.ended(pA)); assertFalse(c.ended(pB))
        assertEquals(1, first.last().revocationLocked); assertEquals(0, first.sumOf { it.failed })
        assertEquals(listOf(held.operationId, unavailable.operationId).sorted(), c.sentFor(pA).sorted())
        assertEquals(listOf(other.operationId), c.sentFor(pB))
        // Locked for every use except delivery; nothing removed: the eligible operation is still pending.
        assertThrows(EnterpriseAccessEndedLocked::class.java) { runBlocking { f.store.record(pA, A) } }
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { c.save(pA, 103) } }
        assertTrue(c.material(pA).isNotEmpty())
        assertFalse(A in c.reviews, "A locked partition still polled review results")
        assertFalse(A in c.uploads, "A locked partition still uploaded attachments")
        assertTrue(B in c.reviews && B in c.uploads, "The other organization keeps its ordinary follow-up")
        // A restarted process keeps the lock and keeps delivering the same eligible operation.
        c.later()
        val second = c.runUntilQuiet()
        assertEquals(listOf(held.operationId, unavailable.operationId, unavailable.operationId).sorted(), c.sentFor(pA).sorted())
        assertEquals(1, second.sumOf { it.revocationLocked }); assertTrue(c.material(pA).isNotEmpty())
        assertEquals(1, c.contacts.size, "An existing lock needs no new contact")
        // The server's final answer for the last operation lets the partition be erased; nothing else is touched.
        c.answer = { p, ops -> EnterpriseOperationDelivery(ops.map { if (p == pA) ack(it, "REJECTED", "MEMBERSHIP_ENDED") else ack(it, "ACCEPTED", null) }) }
        c.later()
        val third = c.runUntilQuiet()
        assertEquals(1, third.sumOf { it.revocationCleaned })
        assertTrue(c.material(pA).isEmpty(), "Every key and marker of the revoked partition is gone")
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { f.store.record(pA, A) } }
        assertArrayEquals(byteArrayOf(2), f.store.record(pB, A)!!.envelope)
        assertTrue(c.material(pB).isNotEmpty())
        assertArrayEquals(byteArrayOf(3), c.foreignRecord())
        val sentBefore = c.sentFor(pA).size
        c.later(); c.runUntilQuiet()
        assertEquals(sentBefore, c.sentFor(pA).size, "An erased partition sent work again")
        assertFalse(f.erasure.cleanupRequired)
    }

    /** B-BR-EMP-538: the window never reopens, so the remaining work is counted, never sent after it closed. */
    @Test fun aClosedIntakeWindowErasesAndTheRemainingWorkIsNeverSentAgain(): Unit = runBlocking {
        val c = Cleanup(); val f = c.f
        f.confirm(pA); f.confirm(pB)
        val closed = c.save(pA, 111); c.save(pA, 112); c.save(pB, 211)
        c.verdict = { EnterpriseContactReply.Lock(it.existingEnrollment!!, A, 10_500) }
        c.answer = { p, ops -> EnterpriseOperationDelivery(ops.map {
            if (p != pA) ack(it, "ACCEPTED", null)
            else if (it.operationId == closed.operationId) ack(it, "REJECTED", "INTAKE_WINDOW_CLOSED") else ack(it, "RETRY", "UNAVAILABLE")
        }) }
        val results = c.runUntilQuiet()
        assertEquals(1, results.sumOf { it.revocationCleaned }, "A closed window with unsent work still erases")
        assertTrue(c.material(pA).isEmpty())
        val sent = c.sentFor(pA).size
        c.later(); c.runUntilQuiet()
        assertEquals(sent, c.sentFor(pA).size)
        assertTrue(c.material(pB).isNotEmpty())
    }

    /** BUC-EMP-276; negative "lost device still sends work". */
    @Test fun aLostDeviceErasesOnlyThatPartitionAndNeverSendsItsWorkAgain(): Unit = runBlocking {
        val c = Cleanup(); val f = c.f
        f.confirm(pA); f.confirm(pB); c.seedForeignAccount()
        f.store.put(EnterpriseLocalRecord(pB, A, byteArrayOf(2)))
        c.save(pA, 121); c.save(pA, 122); c.save(pB, 221)
        c.verdict = { EnterpriseContactReply.Erase(it.existingEnrollment!!, A, 10_500) }
        c.answer = { p, ops -> if (p == pA) throw EnterpriseQueueDeliveryFailure(revocationHint = true)
            else EnterpriseOperationDelivery(ops.map { ack(it, "ACCEPTED", null) }) }
        val results = c.runUntilQuiet()
        assertEquals(1, c.contacts.size)
        assertEquals(1, results.sumOf { it.revocationCleaned })
        assertEquals(2, c.sentFor(pA).size, "Only the one refused delivery before the verdict")
        assertTrue(c.material(pA).isEmpty())
        repeat(3) { c.later(); c.runUntilQuiet() }
        assertEquals(2, c.sentFor(pA).size, "The lost partition's queued work was sent again")
        assertEquals(1, c.contacts.size)
        assertArrayEquals(byteArrayOf(2), f.store.record(pB, A)!!.envelope)
        assertArrayEquals(byteArrayOf(3), c.foreignRecord())
    }

    /** Negative "offline cleanup overclaim": no verdict, no change and no state. */
    @Test fun offlineGenericRefusalsActiveAndForeignVerdictsNeverEraseLockOrClaim(): Unit = runBlocking {
        val c = Cleanup(); val f = c.f
        f.confirm(pA); f.store.put(EnterpriseLocalRecord(pA, A, byteArrayOf(1)))
        c.save(pA, 131)
        // Disconnected: no hint, no contact, nothing claimed.
        c.answer = { _, _ -> throw EnterpriseQueueDeliveryFailure() }
        var results = c.runUntilQuiet()
        assertEquals(0, c.contacts.size)
        // A hint whose contact is refused, unavailable, ACTIVE or names another enrollment changes nothing.
        c.answer = { _, _ -> throw EnterpriseQueueDeliveryFailure(revocationHint = true) }
        val verdicts = listOf<(EnterpriseContactRequest) -> EnterpriseContactReply>(
            { throw EnterpriseProtectionRefused(EnterpriseAccessError.ACCESS_DENIED) },
            { error("synthetic source outage") },
            { testGrant(pA, A, 10_500) },
            { EnterpriseContactReply.Erase(EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(pA, B), id(777), 1), A, 10_500) },
            { EnterpriseContactReply.Lock(EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(pA, B), id(777), 1), A, 10_500) },
            { EnterpriseContactReply.Erase(it.existingEnrollment!!, id(778), 10_500) },
        )
        for (verdict in verdicts) {
            c.verdict = verdict; c.later()
            results = results + c.runUntilQuiet()
        }
        assertEquals(verdicts.size, c.contacts.size)
        assertEquals(0, results.sumOf { it.revocationLocked + it.revocationPending + it.revocationCleaned })
        assertFalse(c.ended(pA))
        assertArrayEquals(byteArrayOf(1), f.store.record(pA, A)!!.envelope)
        assertFalse(f.erasure.cleanupRequired)
        assertTrue(c.sentFor(pA).isNotEmpty())
    }

    /** Negative "interrupted cleanup exposes data": denied, resumable and CLEANED only once verified. */
    @Test fun anInterruptedCleanupStaysDeniedAcrossRestartAndCompletesBeforeClaimingRemoval(): Unit = runBlocking {
        val c = Cleanup(); val f = c.f
        f.confirm(pA); f.confirm(pB)
        f.store.put(EnterpriseLocalRecord(pA, A, byteArrayOf(1))); f.store.put(EnterpriseLocalRecord(pB, A, byteArrayOf(2)))
        c.verdict = { EnterpriseContactReply.Lock(it.existingEnrollment!!, A, 10_500) }
        assertThrows(EnterpriseAccessEndedLocked::class.java) { runBlocking { f.confirm(pA, store = c.store) } }
        f.keys.failAliases = setOf(f.payloadAlias(pA))
        val results = c.runUntilQuiet()
        assertEquals(1, results.sumOf { it.revocationPending }); assertEquals(0, results.sumOf { it.revocationCleaned })
        assertTrue(f.erasure.cleanupRequired)
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { f.store.record(pA, A) } }
        // A new process: still denied, never discovered for delivery, never readable.
        val restarted = VaultFixture(root.resolve("vault"), f.keys)
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { restarted.store.record(pA, A) } }
        assertEquals(listOf(exact(pB)), restarted.store.discover(ProtectedQueueFixture.account(restarted)).enrollments)
        f.keys.failAliases = emptySet()
        val janitor = EnterpriseVaultJanitor(restarted.files, restarted.keys, restarted.fence, restarted.erasure, { restarted.now })
        assertTrue(janitor.runOnce().complete)
        assertFalse(restarted.erasure.cleanupRequired)
        assertTrue(c.material(pA).isEmpty())
        assertArrayEquals(byteArrayOf(2), restarted.store.record(pB, A)!!.envelope)
    }

    /** B-BR-EMP-538 at selection: LOCK is typed, locks reads, status and new work, keeps delivery, survives restart. */
    @Test fun aLockAtSelectionKeepsDeliveryButRefusesReadsStatusAndNewWorkAcrossRestart(): Unit = runBlocking {
        val c = Cleanup(); val f = c.f
        f.confirm(pA); f.store.put(EnterpriseLocalRecord(pA, A, byteArrayOf(1)))
        val pending = c.save(pA, 141)
        c.verdict = { EnterpriseContactReply.Lock(it.existingEnrollment!!, A, 10_500) }
        val locked = assertThrows(EnterpriseAccessEndedLocked::class.java) { runBlocking { f.confirm(pA, store = c.store) } }
        assertTrue(EnterpriseLocalDenied::class.java.isInstance(locked), "Every existing denial path still applies")
        assertTrue(c.ended(pA)); assertTrue(c.material(pA).isNotEmpty())
        val restarted = Cleanup(VaultFixture(root.resolve("vault"), f.keys))
        assertThrows(EnterpriseAccessEndedLocked::class.java) { runBlocking { restarted.f.store.record(pA, A) } }
        val attempt = ProtectedQueueFixture.attempt(restarted.f, pA)
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { restarted.queue.stats(attempt) } }
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { restarted.save(pA, 142) } }
        assertEquals(listOf(pending.operationId), restarted.queue.pending(attempt).map { it.operation.operationId })
        restarted.answer = { _, ops -> EnterpriseOperationDelivery(ops.map { ack(it, "ACCEPTED_FOR_REVIEW", "ACCESS_ENDED_BEFORE_RECEIPT") }) }
        restarted.verdict = { EnterpriseContactReply.Lock(it.existingEnrollment!!, A, 10_600) }
        val results = restarted.runUntilQuiet()
        assertEquals(listOf(pending.operationId), restarted.sentFor(pA))
        assertEquals(1, results.sumOf { it.revocationCleaned })
        assertTrue(restarted.material(pA).isEmpty())
    }
}
