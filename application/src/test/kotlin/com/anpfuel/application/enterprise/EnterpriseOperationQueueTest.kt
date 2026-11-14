package com.anpfuel.application.enterprise

import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.sync.*
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseOperationQueueTest {
    private fun id(n: Int) = "7c000000-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val partition = EnterprisePartition.of(id(1), "https://queue.example.invalid", id(3))
    private fun op(n: Int, deps: List<Int> = emptyList()): EnterpriseOperation = EnterpriseOperation.seal("""{"schema_version":1,"operation_id":"${id(n)}","owner_account_id":"${id(1)}","api_origin":"${partition.origin}","org_id":"${id(3)}","membership_id":"${id(4)}","enrollment_id":"${id(5)}","type":"validation.queue","operation_schema":1,"entity":{"type":"record","id":"${id(100)}","expected_revision":null},"occurred_at":"2026-11-13T00:00:00Z","entry_zone":"UTC","device_offset_seconds":0,"depends_on":[${deps.joinToString { "\"${id(it)}\"" }}],"soft_references":[],"attachments":[],"payload":{"local_operation":"${id(n)}"}}""".toByteArray(), SyncBinding.of(id(1), partition.origin, id(3)))
    private fun ack(op: EnterpriseOperation, result: String = "ACCEPTED", code: String? = null): OperationAcknowledgement {
        val final = result != "RETRY"
        val revision = if (result == "ACCEPTED") "1" else "null"
        val received = if (final) "\"2026-11-13T00:00:01Z\"" else "null"
        val review = if (result == "ACCEPTED_FOR_REVIEW") "\"${id(70)}\"" else "null"
        return OperationAcknowledgement.parse("""{"schema_version":1,"operation_id":"${op.operationId}","payload_digest":"${op.payloadDigest}","result":"$result","code":${code?.let { "\"$it\"" } ?: "null"},"entity_revision":$revision,"received_at":$received,"review_item_id":$review,"warnings":[]}""".toByteArray(), op)
    }
    private inner class Fixture {
        var retained = true
        val account = EnterpriseQueueAccountLease.capture(EnterpriseLocalIdentity(id(1), partition.origin, id(8)), 2, 3, id(9),
            proofBudget = SyncDeliveryBudget.fromProof(id(8), "short")) { retained }
        val attempt = account.forEnrollment(EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(partition, id(4)), id(5), 1))
        val entries = linkedMapOf<String, EnterpriseQueueEntry>()
        val records = linkedMapOf<String, EnterpriseLocalRecord>()
        var projected = 0
        var acknowledged = 0
        var sent = mutableListOf<List<String>>()
        var failAck: String? = null
        val now = EnterpriseDeviceClock("boot", 1000, 10000)
        val source = object : EnterpriseQueueSource {
            override suspend fun account() = account
            override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt {
                account.assertCurrent(); if (account !== this@Fixture.account) throw EnterpriseLocalDenied(); return attempt
            }
        }
        fun transaction(values: MutableMap<String, EnterpriseLocalRecord>) = object : EnterpriseQueueTransaction {
            override fun record(id: String) = values[id]
            override fun put(record: EnterpriseLocalRecord) { require(record.partition == partition); values[record.id] = record }
        }
        val queue = object : EnterpriseOperationQueue {
            override suspend fun save(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, project: (EnterpriseQueueTransaction) -> Unit): Boolean {
                attempt.assertCurrent()
                entries[operation.operationId]?.let { require(it.operation.sameIntent(operation)); return false }
                val changed = records.toMutableMap(); project(transaction(changed)); attempt.assertCurrent()
                entries[operation.operationId] = EnterpriseQueueEntry(operation); records.clear(); records.putAll(changed); return true
            }
            override suspend fun pending(attempt: EnterpriseQueueAttempt) = entries.values.filter { it.acknowledgement == null }
            override suspend fun entry(attempt: EnterpriseQueueAttempt, operationId: String) = entries[operationId]
            override suspend fun acknowledge(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, ack: OperationAcknowledgement, project: (EnterpriseQueueTransaction) -> Unit): Boolean {
                attempt.assertCurrent()
                val existing = entries.getValue(operation.operationId)
                existing.acknowledgement?.let { require(it.canonicalBytes.contentEquals(ack.canonicalBytes)); return false }
                if (failAck == operation.operationId) throw EnterprisePersistenceUnavailable()
                val changed = records.toMutableMap(); project(transaction(changed)); attempt.assertCurrent()
                entries[operation.operationId] = EnterpriseQueueEntry(operation, existing.retry, ack); records.clear(); records.putAll(changed); return true
            }
            override suspend fun retry(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, expected: QueueRetryState, next: QueueRetryState): Boolean {
                attempt.assertCurrent(); val old = entries.getValue(operation.operationId)
                if (old.acknowledgement != null || old.retry.attempts != expected.attempts || old.retry.clock != expected.clock ||
                    old.retry.delayMillis != expected.delayMillis) return false
                entries[operation.operationId] = EnterpriseQueueEntry(old.operation, next, old.acknowledgement)
                return true
            }
            override suspend fun stats(attempt: EnterpriseQueueAttempt) = EnterpriseQueueStats(entries.size, 0, 0, entries.size, 2048)
        }
        val producer = object : EnterpriseQueueProducer {
            override fun descriptor() = EnterpriseQueueProducerDescriptor("validation.queue")
            override fun save(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation) {
                projected++; tx.put(EnterpriseLocalRecord(partition, operation.entity.reference.id, operation.operationId.toByteArray()))
            }
            override fun acknowledge(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, ack: OperationAcknowledgement) {
                acknowledged++
                // This real local fixture owns its version token. Older ACKs
                // record receipt without overwriting an independently newer edit.
                val current = tx.record(operation.entity.reference.id)
                if (current?.envelope?.contentEquals(operation.operationId.toByteArray()) == true)
                    tx.put(EnterpriseLocalRecord(partition, operation.entity.reference.id, "accepted:${operation.operationId}".toByteArray()))
            }
        }
        fun service(reply: suspend (List<EnterpriseOperation>) -> EnterpriseOperationDelivery = { ops -> EnterpriseOperationDelivery(ops.map { ack(it) }) },
            registry: EnterpriseQueueRegistry = EnterpriseQueueRegistry(producer)) = EnterpriseQueueService(source, queue,
            EnterpriseOperationTransport { captured, operations -> captured.assertCurrent(); sent.add(operations.map { it.operationId }); reply(operations) },
            registry, { now }, { 0 })
    }

    @Test fun saveRequiresActualProducerAndCommitsProjectionBeforeReturningReplay(): Unit = runTest {
        val f = Fixture(); val operation = op(10)
        assertThrows(EnterprisePersistenceUnavailable::class.java) { kotlinx.coroutines.runBlocking { f.service(registry = EnterpriseQueueRegistry()).save(partition, operation) } }
        assertTrue(f.entries.isEmpty()); assertEquals(0, f.projected)
        val service = f.service()
        assertTrue(service.save(partition, operation)); assertFalse(service.save(partition, operation))
        assertEquals(1, f.projected); assertArrayEquals(operation.operationId.toByteArray(), f.records.getValue(id(100)).envelope)
        f.retained = false
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { service.save(partition, op(11)) } }
        assertEquals(1, f.entries.size)
    }

    @Test fun dispatchPreflightsWholeAnswerAndNeverProjectsStaleAttempt(): Unit = runTest {
        val f = Fixture(); val first = op(10); val second = op(11)
        f.service().save(partition, first); f.service().save(partition, second)
        val malformed = f.service({ EnterpriseOperationDelivery(listOf(ack(first), ack(first))) })
        val bad = malformed.dispatch(f.account, partition)
        assertEquals(0, bad.completed); assertEquals(0, f.acknowledged)
        assertTrue(f.entries.values.all { it.acknowledgement == null })
        val stale = Fixture(); stale.service().save(partition, first)
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking {
            stale.service({ operations -> stale.retained = false; EnterpriseOperationDelivery(operations.map { ack(it) }) }).dispatch(stale.account, partition)
        } }
        assertEquals(0, stale.acknowledged)
    }

    @Test fun oldReceiptCannotOverwriteNewerLocalEditAndOneProjectionFailureDoesNotBlockOthers(): Unit = runTest {
        val f = Fixture(); val old = op(10); val newer = op(11)
        val service = f.service(); service.save(partition, old); service.save(partition, newer)
        f.failAck = old.operationId
        val result = service.dispatch(f.account, partition)
        assertEquals(1, result.completed); assertNull(f.entries.getValue(old.operationId).acknowledgement)
        assertNotNull(f.entries.getValue(newer.operationId).acknowledgement)
        f.failAck = null
        val due = f.entries.getValue(old.operationId)
        f.entries[old.operationId] = EnterpriseQueueEntry(old, QueueRetryState())
        service.dispatch(f.account, partition)
        assertArrayEquals("accepted:${newer.operationId}".toByteArray(), f.records.getValue(id(100)).envelope)
        assertTrue(due.retry.attempts > 0)
    }

    @Test fun cyclesStayRetainedAndUnrelatedOperationsProgressInParentOrder(): Unit = runTest {
        val f = Fixture(); val cycle = listOf(op(10, listOf(11)), op(11, listOf(10)))
        val child = op(13, listOf(12)); val parent = op(12); val service = f.service()
        for (operation in cycle + child + parent) service.save(partition, operation)
        val result = service.dispatch(partition)
        assertEquals(2, result.attention); assertEquals(2, result.completed)
        assertEquals(listOf(parent.operationId, child.operationId), f.sent.single())
        assertTrue(cycle.all { f.entries.getValue(it.operationId).acknowledgement == null })
    }

    @Test fun reviewReceiptStaysOriginalWhileChildRetriesAgainstServerReadiness(): Unit = runTest {
        val f = Fixture(); val parent = op(10); val child = op(11, listOf(10))
        val service = f.service({ operations -> EnterpriseOperationDelivery(operations.map {
            if (it.operationId == parent.operationId) ack(it, "ACCEPTED_FOR_REVIEW", "ORPHAN_REFERENCE") else ack(it, "RETRY", "DEPENDENCY_PENDING")
        }) })
        service.save(partition, child); service.save(partition, parent)
        val first = service.dispatch(partition)
        assertEquals(1, first.completed); assertEquals(1, first.retryable)
        val original = f.entries.getValue(parent.operationId).acknowledgement!!.canonicalBytes
        f.entries[child.operationId] = EnterpriseQueueEntry(child)
        assertEquals(1, f.service().dispatch(partition).completed)
        assertEquals(listOf(child.operationId), f.sent.last())
        assertArrayEquals(original, f.entries.getValue(parent.operationId).acknowledgement!!.canonicalBytes)
    }

    @Test fun retryHonorsServerFloorAndWorkerCannotAdoptAnotherAccountLease(): Unit = runTest {
        val f = Fixture(); val operation = op(10)
        val service = f.service({ throw EnterpriseQueueDeliveryFailure(120000) })
        service.save(partition, operation)
        val result = service.dispatch(f.account, partition)
        assertEquals(1, result.retryable); assertEquals(120000L, f.entries.getValue(operation.operationId).retry.delayMillis)
        assertEquals(1L, f.entries.getValue(operation.operationId).retry.attempts)
        assertTrue(f.entries.values.all { it.acknowledgement == null })
        val replacement = EnterpriseQueueAccountLease.capture(EnterpriseLocalIdentity(id(1), partition.origin, id(8)), 2, 3, id(20)) { true }
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { service.dispatch(replacement, partition) } }
    }

    @Test fun offlineSaveNeverRefreshesAndEveryDeliveredBatchHonorsCapturedLimits(): Unit = runTest {
        val f = Fixture(); var localCaptures = 0; var deliveryCaptures = 0
        val source = object : EnterpriseQueueSource by f.source {
            override suspend fun account(): EnterpriseQueueAccountLease { localCaptures++; return f.account }
            override suspend fun deliveryAccount(): EnterpriseQueueAccountLease { deliveryCaptures++; return f.account }
        }
        val sizes = mutableListOf<Int>()
        val service = EnterpriseQueueService(source, f.queue, EnterpriseOperationTransport { _, ops ->
            sizes.add(ops.size); EnterpriseOperationDelivery(ops.map { ack(it) })
        }, EnterpriseQueueRegistry(f.producer), { f.now }, { 0 }, SyncLimits.trusted(maxOperations = 1))
        service.save(partition, op(10)); service.save(partition, op(11))
        assertEquals(2, localCaptures); assertEquals(0, deliveryCaptures)
        val first = service.dispatch(partition)
        assertEquals(1, first.completed); assertEquals(1, first.retryable); assertEquals(0L, first.retryAfterMillis)
        assertEquals(1, service.dispatch(partition).completed)
        assertEquals(listOf(1, 1), sizes); assertEquals(2, localCaptures); assertEquals(2, deliveryCaptures)
    }

    @Test fun sourceCannotSubstituteAnEquivalentNewLeaseAndPendingCannotRetagIntent(): Unit = runTest {
        val f = Fixture()
        val replacement = EnterpriseQueueAccountLease.capture(f.account.identity, 2, 3, id(30)) { true }
        val source = object : EnterpriseQueueSource by f.source {
            override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition) = replacement.forEnrollment(f.attempt.enrollment)
        }
        val service = EnterpriseQueueService(source, f.queue, EnterpriseOperationTransport { _, _ -> error("Unexpected network") },
            EnterpriseQueueRegistry(f.producer), { f.now }, { 0 })
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { service.save(partition, op(10)) } }
        assertTrue(f.entries.isEmpty())
        val root = CanonicalJson.parse(op(10).canonicalBytes) as SyncJson.Object
        val foreign = EnterpriseOperation.seal(CanonicalJson.encode(SyncJson.Object(root.fields - "payload_digest" +
            ("membership_id" to SyncJson.Text(id(40))))), op(10).binding)
        val queue = object : EnterpriseOperationQueue by f.queue {
            override suspend fun pending(attempt: EnterpriseQueueAttempt) = listOf(EnterpriseQueueEntry(foreign))
        }
        val retag = EnterpriseQueueService(f.source, queue, EnterpriseOperationTransport { _, _ -> error("Unexpected network") },
            EnterpriseQueueRegistry(f.producer), { f.now }, { 0 })
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { retag.dispatch(partition) } }
    }

    @Test fun privatePortFailureIsRedactedAndCancellationRemainsCancellation(): Unit = runTest {
        val f = Fixture(); val sentinel = "PRIVATE-QUEUE-CONTENT-MUST-NOT-LEAK"
        val queue = object : EnterpriseOperationQueue by f.queue {
            override suspend fun pending(attempt: EnterpriseQueueAttempt): List<EnterpriseQueueEntry> = throw IllegalStateException(sentinel)
        }
        val service = EnterpriseQueueService(f.source, queue, EnterpriseOperationTransport { _, _ -> error("Unexpected network") },
            EnterpriseQueueRegistry(f.producer), { f.now }, { 0 })
        val failure = assertThrows(EnterprisePersistenceUnavailable::class.java) { kotlinx.coroutines.runBlocking { service.dispatch(partition) } }
        assertFalse(failure.toString().contains(sentinel)); assertNull(failure.cause)
        val cancelled = object : EnterpriseOperationQueue by f.queue {
            override suspend fun pending(attempt: EnterpriseQueueAttempt): List<EnterpriseQueueEntry> = throw kotlinx.coroutines.CancellationException("Synthetic cancellation")
        }
        val cancellation = EnterpriseQueueService(f.source, cancelled, EnterpriseOperationTransport { _, _ -> error("Unexpected network") },
            EnterpriseQueueRegistry(f.producer), { f.now }, { 0 })
        assertThrows(kotlinx.coroutines.CancellationException::class.java) { kotlinx.coroutines.runBlocking { cancellation.dispatch(partition) } }
    }

    @Test fun delayedAndUnknownWorkRemainsUnsentWithoutInventingAReadinessDecision(): Unit = runTest {
        val f = Fixture(); val operation = op(10)
        f.service().save(partition, operation)
        f.entries[operation.operationId] = EnterpriseQueueEntry(operation, QueueRetryState(1, f.now, 100000))
        val delayed = f.service().dispatch(partition)
        assertEquals(1, delayed.retryable); assertEquals(100000L, delayed.retryAfterMillis); assertTrue(f.sent.isEmpty())
        val unavailable = f.service(registry = EnterpriseQueueRegistry()).dispatch(partition)
        assertEquals(1, unavailable.attention); assertEquals(0, unavailable.attempted)
        assertNull(f.entries.getValue(operation.operationId).acknowledgement)
        assertEquals(100000L, f.entries.getValue(operation.operationId).retry.delayMillis)
        assertThrows(IllegalArgumentException::class.java) { EnterpriseQueueRegistry(f.producer, f.producer) }
        for (type in listOf("single", "bad..type", "Upper.type", "valid._bad", "a.a__b", "a.a_"))
            assertThrows(IllegalArgumentException::class.java) { EnterpriseQueueProducerDescriptor(type) }
        // Long trusted descriptors use a bounded scanner, not recursive regex backtracking.
        EnterpriseQueueProducerDescriptor("a.a" + "_a".repeat(2000))
    }

    @Test fun custodyAllowedAfterReconfirmationRebasesRetryDelayOnceAndEventuallyConverges(): Unit = runTest {
        for (oldClock in listOf(EnterpriseDeviceClock("old-boot", 9000, 20000), EnterpriseDeviceClock("boot", 9000, 20000))) {
            val f = Fixture(); val operation = op(10); var now = f.now
            f.service().save(partition, operation)
            f.entries[operation.operationId] = EnterpriseQueueEntry(operation, QueueRetryState(4, oldClock, 120000))
            val service = EnterpriseQueueService(f.source, f.queue,
                EnterpriseOperationTransport { _, operations -> f.sent.add(operations.map { it.operationId }); EnterpriseOperationDelivery(operations.map { ack(it) }) },
                EnterpriseQueueRegistry(f.producer), { now }, { 0 })
            // The real queue source has already reopened only after protected
            // custody's clock/reboot reconfirmation. Cadence is not authority.
            val deferred = service.dispatch(partition)
            assertEquals(1, deferred.retryable); assertTrue(f.sent.isEmpty())
            val rebased = f.entries.getValue(operation.operationId).retry
            assertEquals(now, rebased.clock); assertEquals(4L, rebased.attempts); assertEquals(120000L, rebased.delayMillis)
            assertEquals(120000L, deferred.retryAfterMillis)
            now = EnterpriseDeviceClock("boot", 121000, 130000)
            assertEquals(1, service.dispatch(partition).completed)
        }
    }

    @Test fun actualShortDeliveryProofFitsExactTrustedBoundaryAndOneByteLessRetainsWork(): Unit = runTest {
        val operation = op(10)
        val exact = SyncBatch.encode(id(8), "short", listOf(operation)).size
        for (fits in listOf(true, false)) {
            val f = Fixture(); f.service().save(partition, operation)
            val limits = SyncLimits.trusted(maxBatchBytes = exact - if (fits) 0 else 1, maxOperationBytes = operation.canonicalBytes.size)
            var sent = false
            val service = EnterpriseQueueService(f.source, f.queue, EnterpriseOperationTransport { _, operations ->
                SyncBatch.encode(id(8), "short", operations, limits)
                sent = true
                EnterpriseOperationDelivery(operations.map { ack(it) })
            }, EnterpriseQueueRegistry(f.producer), { f.now }, { 0 }, limits)
            val result = service.dispatch(partition)
            assertEquals(fits, sent)
            assertEquals(if (fits) 1 else 0, result.completed)
            assertEquals(!fits, f.entries.getValue(operation.operationId).acknowledgement == null)
        }
    }

    @Test fun olderDeliveryCannotReplaceANewerProtectedRetryFloor(): Unit = runTest {
        val f = Fixture(); val operation = op(10)
        f.service().save(partition, operation)
        val newer = QueueRetryState(1, f.now, 3600000)
        val service = f.service({
            // A second still-current worker commits its retry while this
            // older request is in flight. UUID/time sorting is not a CAS.
            f.entries[operation.operationId] = EnterpriseQueueEntry(operation, newer)
            throw EnterpriseQueueDeliveryFailure(30000)
        })
        val result = service.dispatch(partition)
        val retained = f.entries.getValue(operation.operationId).retry
        assertEquals(newer.attempts, retained.attempts); assertEquals(newer.clock, retained.clock)
        assertEquals(3600000L, retained.delayMillis)
        assertEquals(3600000L, result.retryAfterMillis)
        assertNull(f.entries.getValue(operation.operationId).acknowledgement)
    }

    @Test fun noSendResultCannotExposePrivateCountsAfterSourceReplacement(): Unit = runTest {
        for (kind in listOf("unknown", "delayed", "cycle", "empty")) {
            val f = Fixture(); val operation = op(10)
            if (kind != "empty") f.service().save(partition, operation)
            if (kind == "delayed") f.entries[operation.operationId] = EnterpriseQueueEntry(operation, QueueRetryState(1, f.now, 120000))
            if (kind == "cycle") {
                f.entries.clear()
                f.service().save(partition, op(10, listOf(11)))
                f.service().save(partition, op(11, listOf(10)))
            }
            val registry = if (kind == "unknown") EnterpriseQueueRegistry() else EnterpriseQueueRegistry(f.producer)
            val service = EnterpriseQueueService(f.source, f.queue,
                EnterpriseOperationTransport { _, _ -> error("No transport should run") }, registry,
                { f.retained = false; f.now }, { 0 })
            assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { service.dispatch(f.account, partition) } }
            assertTrue(f.entries.values.all { it.acknowledgement == null })
            assertTrue(f.sent.isEmpty())
        }
    }

    @Test fun cancelledNoSendAttemptCannotReturnEvenWhileItsSourceIsRetained(): Unit = runTest {
        val f = Fixture(); f.service().save(partition, op(10))
        val job = kotlinx.coroutines.Job()
        var returned = false
        val service = EnterpriseQueueService(f.source, f.queue,
            EnterpriseOperationTransport { _, _ -> error("No transport should run") }, EnterpriseQueueRegistry(),
            { job.cancel(); f.now }, { 0 })
        try {
            kotlinx.coroutines.withContext(job) {
                service.dispatch(f.account, partition)
                returned = true
            }
            fail("Cancelled dispatch returned successfully")
        } catch (_: kotlinx.coroutines.CancellationException) {
            assertFalse(returned, "The service must refuse success before the surrounding cancellation boundary")
        } finally { job.cancel() }
        assertTrue(f.entries.values.all { it.acknowledgement == null })
    }
}
