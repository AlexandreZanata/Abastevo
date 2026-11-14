package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.application.enterprise.sync.*
import com.anpfuel.data.enterprise.ProtectedQueueFixture.attempt
import com.anpfuel.data.enterprise.ProtectedQueueFixture.id
import com.anpfuel.data.enterprise.ProtectedQueueFixture.operation
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseSyncStatusStore
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.sync.*
import java.io.ByteArrayOutputStream
import java.io.DataOutputStream
import java.nio.file.Path
import java.security.MessageDigest
import java.util.concurrent.Callable
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

/** Protected V3 review persistence, historical V1/V2 recovery and outcome-bound visibility. */
class ProtectedEnterpriseReviewQueueTest {
    init {
        val tests = ProtectedEnterpriseReviewQueueTest::class.java.declaredMethods.filter { it.isAnnotationPresent(Test::class.java) }
        check(tests.size == 8 && tests.all { it.returnType == Void.TYPE }) { "Protected review tests must be discoverable void methods" }
    }

    @TempDir lateinit var root: Path
    private fun generations(f: VaultFixture) = EnterpriseVaultGenerations(f.files, f.keys)
    private fun head(f: VaultFixture, p: EnterprisePartition = partition()) = generations(f).head(EnterpriseVaultCodec.token(p))
    private fun sha(bytes: ByteArray) = "sha256:" + MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it.toInt() and 255) }
    private val reviewItem = id(800)
    private fun receipt(op: EnterpriseOperation, result: String = "ACCEPTED_FOR_REVIEW", review: String = reviewItem): OperationAcknowledgement {
        val code = when (result) { "ACCEPTED" -> "null"; "ACCEPTED_FOR_REVIEW" -> "\"PERMISSION_CHANGED\""; else -> "\"MALFORMED\"" }
        val revision = if (result == "ACCEPTED") "1" else "null"
        val reviewValue = if (result == "ACCEPTED_FOR_REVIEW") "\"$review\"" else "null"
        return OperationAcknowledgement.parse("""{"schema_version":1,"operation_id":"${op.operationId}","payload_digest":"${op.payloadDigest}","result":"$result","code":$code,"entity_revision":$revision,"received_at":"2026-11-13T12:00:01Z","review_item_id":$reviewValue,"warnings":[]}""".toByteArray(), op)
    }
    private fun decision(op: EnterpriseOperation, outcome: String = "ACCEPTED", review: String = reviewItem, resolutionId: String = id(801)) =
        ReviewResolution.create(ReviewResolutionSnapshot(op.operationId, op.payloadDigest, review, resolutionId, 2, outcome,
            if (outcome == "REFUSED") "Synthetic private refusal 😀" else "", "2026-11-13T12:05:00.123456789Z", if (outcome == "ACCEPTED") 2 else null))
    private suspend fun seed(f: VaultFixture, op: EnterpriseOperation, ack: OperationAcknowledgement?) {
        val queue = ProtectedEnterpriseQueue(f.store)
        val captured = attempt(f, EnterprisePartition.of(op.binding.ownerAccountId, op.binding.apiOrigin, op.binding.organizationId))
        queue.save(captured, op) { it.put(EnterpriseLocalRecord(captured.partition, op.operationId, ("SYNTHETIC-PROJECTION-" + op.operationId).toByteArray())) }
        if (ack != null) queue.acknowledge(captured, op, ack) { }
    }
    /** Historical accepted V1/V2 queue wire, independent of the current encoder. */
    private fun historical(op: EnterpriseOperation, ack: OperationAcknowledgement?, version: Int, retry: QueueRetryState = QueueRetryState(),
        dismissed: Boolean = false): ByteArray = ByteArrayOutputStream().also { bytes ->
        DataOutputStream(bytes).use { out ->
            out.writeInt(0x51554555); out.writeInt(version)
            val canonical = op.canonicalBytes; out.writeInt(canonical.size); out.write(canonical)
            val raw = ack?.canonicalBytes; out.writeInt(raw?.size ?: -1); if (raw != null) out.write(raw)
            out.writeLong(retry.attempts); out.writeBoolean(retry.clock != null)
            retry.clock?.let { out.writeUTF(it.boot); out.writeLong(it.elapsedMillis); out.writeLong(it.wallMillis) }
            out.writeLong(retry.delayMillis)
            if (version == 2) out.writeBoolean(dismissed)
        }
    }.toByteArray()
    /** Independent V3 writer used only to prove strict decoding of corrupted frames. */
    private fun v3(op: EnterpriseOperation, ack: OperationAcknowledgement?, dismissedKind: Int, resolution: ByteArray?,
        reviewRetry: QueueRetryState = QueueRetryState(), trailing: Boolean = false): ByteArray = ByteArrayOutputStream().also { bytes ->
        DataOutputStream(bytes).use { out ->
            out.writeInt(0x51554555); out.writeInt(3)
            val canonical = op.canonicalBytes; out.writeInt(canonical.size); out.write(canonical)
            val raw = ack?.canonicalBytes; out.writeInt(raw?.size ?: -1); if (raw != null) out.write(raw)
            out.writeLong(0); out.writeBoolean(false); out.writeLong(0)
            out.writeByte(dismissedKind)
            out.writeInt(resolution?.size ?: -1); if (resolution != null) out.write(resolution)
            out.writeLong(reviewRetry.attempts); out.writeBoolean(reviewRetry.clock != null)
            reviewRetry.clock?.let { out.writeUTF(it.boot); out.writeLong(it.elapsedMillis); out.writeLong(it.wallMillis) }
            out.writeLong(reviewRetry.delayMillis)
            if (trailing) out.writeByte(0)
        }
    }.toByteArray()
    private fun replace(f: VaultFixture, op: EnterpriseOperation, bytes: ByteArray, p: EnterprisePartition = partition()) {
        val snapshot = generations(f).load(p)!!
        snapshot.entries["QUEUE:" + op.operationId] = VaultEntry(VaultKind.QUEUE, op.operationId, "", bytes)
        generations(f).commit(snapshot)
    }
    private fun stored(f: VaultFixture, op: EnterpriseOperation, p: EnterprisePartition = partition()): ByteArray =
        generations(f).load(p)!!.entries.getValue("QUEUE:" + op.operationId).bytes
    private fun version(bytes: ByteArray) = java.nio.ByteBuffer.wrap(bytes, 4, 4).int

    @Test fun historicalV1AndV2ReviewFramesRecoverAndMigrateOnlyWhenResolved(): Unit = runBlocking {
        for (format in listOf(1, 2)) {
            val f = VaultFixture(root.resolve("vault-$format")); f.confirm(partition()); f.confirm(partition(org = B))
            val op = operation(operationId = id(100)); val ack = receipt(op)
            val other = operation(p = partition(org = B), operationId = id(101), value = "SYNTHETIC-OTHER-ORG")
            seed(f, op, null); seed(f, other, receipt(other))
            val retry = QueueRetryState(3, f.now, 45_000)
            replace(f, op, historical(op, ack, format, retry))
            val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f)
            val before = head(f); val otherHead = head(f, partition(org = B)); val otherBytes = stored(f, other, partition(org = B))
            val unresolved = queue.unresolvedReviews(captured)
            assertEquals(listOf(op.operationId), unresolved.map { it.operation.operationId })
            assertTrue(queue.pending(captured).isEmpty(), "A final REVIEW became pending delivery")
            assertEquals(before, head(f), "Reading old framing rewrote it")
            assertTrue(queue.resolve(captured, op, ack, decision(op)))
            val migrated = stored(f, op)
            assertEquals(3, version(migrated))
            val entry = queue.entry(captured, op.operationId)!!
            assertArrayEquals(op.canonicalBytes, entry.operation.canonicalBytes)
            assertArrayEquals(ack.canonicalBytes, entry.acknowledgement!!.canonicalBytes, "Original REVIEW bytes changed")
            assertArrayEquals(decision(op).canonicalBytes, entry.resolution!!.canonicalBytes)
            assertTrue(entry.retry.sameAs(retry), "Delivery retry metadata was lost during migration")
            assertTrue(queue.unresolvedReviews(captured).isEmpty())
            assertEquals(otherHead, head(f, partition(org = B))); assertArrayEquals(otherBytes, stored(f, other, partition(org = B)))
            assertEquals(1, queue.unresolvedReviews(attempt(f, partition(org = B))).size, "Another organization's REVIEW was resolved")
        }
    }

    @Test fun resolutionCompareAndSetBindsExactReceiptReviewAndTerminalOutcome(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val op = operation(operationId = id(100)); val ack = receipt(op)
        val accepted = operation(operationId = id(102)); val acceptedAck = receipt(accepted, "ACCEPTED")
        val saved = operation(operationId = id(103))
        seed(f, op, ack); seed(f, accepted, acceptedAck); seed(f, saved, null)
        val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f)
        val start = head(f)
        val otherReceipt = OperationAcknowledgement.parse(String(ack.canonicalBytes).replace("PERMISSION_CHANGED", "ASSET_BUSY").toByteArray(), op)
        for (attemptWrite in listOf<suspend () -> Boolean>(
            { queue.resolve(captured, op, otherReceipt, decision(op)) },
            { queue.resolve(captured, op, ack, decision(op, review = id(899))) },
            { queue.resolve(captured, accepted, acceptedAck, decision(accepted)) },
            { queue.resolve(captured, saved, receipt(saved), decision(saved)) },
        )) {
            try { attemptWrite(); fail<Unit>("An uncorrelated resolution was accepted") }
            catch (_: EnterprisePersistenceUnavailable) { }
            catch (_: EnterpriseOperationReused) { }
        }
        assertEquals(start, head(f), "A refused resolution attempt published a generation")
        assertTrue(queue.resolve(captured, op, ack, decision(op)))
        val committed = head(f)
        assertFalse(queue.resolve(captured, op, ack, decision(op)), "Equal replay must be a no-op")
        assertEquals(committed, head(f))
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { queue.resolve(captured, op, ack, decision(op, "REFUSED")) } }
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { queue.resolve(captured, op, ack, decision(op, resolutionId = id(899))) } }
        assertEquals(committed, head(f))
        assertFalse(queue.reviewRetry(captured, op, ack, QueueRetryState(), QueueRetryState(1, f.now, 30_000)), "A resolved review accepted retry metadata")
        f.fence.markBackground()
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { queue.resolve(captured, op, ack, decision(op)) } }
    }

    @Test fun reviewRetryCasIsSeparateFromDeliveryRetryAndPreservesNewerState(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val op = operation(operationId = id(100)); val ack = receipt(op)
        seed(f, op, ack)
        val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f)
        val deliveryRetry = queue.entry(captured, op.operationId)!!.retry
        val first = QueueRetryState(1, f.now, 30_000)
        assertTrue(queue.reviewRetry(captured, op, ack, QueueRetryState(), first))
        assertEquals(3, version(stored(f, op)))
        val newer = QueueRetryState(2, f.now, 60_000)
        assertTrue(queue.reviewRetry(captured, op, ack, first, newer))
        val committed = head(f)
        assertFalse(queue.reviewRetry(captured, op, ack, first, QueueRetryState(2, f.now, 1)), "A stale writer shortened a newer retry")
        assertTrue(queue.reviewRetry(captured, op, ack, newer, newer), "Equal retry state is a current no-op")
        assertEquals(committed, head(f))
        val entry = queue.entry(captured, op.operationId)!!
        assertTrue(entry.reviewRetry.sameAs(newer)); assertTrue(entry.retry.sameAs(deliveryRetry))
        assertFalse(queue.retry(captured, op, deliveryRetry, QueueRetryState(9, f.now, 1)), "Delivery retry changed a final REVIEW")
        assertTrue(queue.reviewRetry(captured, op, ack, newer, QueueRetryState()))
        assertEquals(2, version(stored(f, op)), "A record without review-only state keeps the minimal frame")
    }

    @Test fun corruptedOrInconsistentV3FramesStayUnavailableWithoutRepair(): Unit = runBlocking {
        val op = operation(operationId = id(100)); val ack = receipt(op)
        val refused = decision(op, "REFUSED").canonicalBytes; val accepted = decision(op).canonicalBytes
        val bad = listOf(
            v3(op, ack, 1, refused), v3(op, ack, 3, refused), v3(op, ack, 0, accepted, trailing = true),
            v3(op, ack, 2, accepted), v3(op, null, 0, accepted), v3(op, receipt(op, "REJECTED"), 0, accepted),
            v3(op, ack, 0, accepted.copyOf().also { it[it.size - 2] = '2'.code.toByte() }),
            v3(op, ack, 0, (" " + String(accepted)).toByteArray()),
            v3(op, ack, 0, decision(op, review = id(899)).canonicalBytes),
            v3(op, ack, 0, accepted, QueueRetryState(1, null, 0)),
        )
        for ((index, frame) in bad.withIndex()) {
            val f = VaultFixture(root.resolve("vault-$index")); f.confirm(partition())
            seed(f, op, ack); replace(f, op, frame)
            val queue = ProtectedEnterpriseQueue(f.store); val before = head(f)
            assertThrows(EnterprisePersistenceUnavailable::class.java, { runBlocking { queue.entry(attempt(f), op.operationId) } }, "frame $index")
            assertEquals(before, head(f)); assertArrayEquals(frame, stored(f, op), "Unknown framing was repaired")
        }
        val f = VaultFixture(root.resolve("vault-good")); f.confirm(partition()); seed(f, op, ack)
        replace(f, op, v3(op, ack, 2, refused))
        val entry = ProtectedEnterpriseQueue(f.store).entry(attempt(f), op.operationId)!!
        assertTrue(entry.dismissedRejected); assertEquals(ReviewResolution.REFUSED, entry.resolution!!.snapshot().outcome)
    }

    @Test fun refusedResolutionDismissalUsesTheEffectiveOutcomeDigest(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val op = operation(operationId = id(100)); val ack = receipt(op)
        val held = operation(operationId = id(101)); val heldAck = receipt(held)
        seed(f, op, ack); seed(f, held, heldAck)
        val queue = ProtectedEnterpriseQueue(f.store); val status = ProtectedEnterpriseSyncStatusStore(f.store); val captured = attempt(f)
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { status.dismissRejected(captured, held.operationId, held.payloadDigest, sha(heldAck.canonicalBytes)) } }
        val refused = decision(op, "REFUSED")
        assertTrue(queue.resolve(captured, op, ack, refused))
        val before = head(f)
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { status.dismissRejected(captured, op.operationId, op.payloadDigest, sha(ack.canonicalBytes)) } }
        assertEquals(before, head(f), "The original REVIEW digest hid a later decision")
        assertTrue(status.dismissRejected(captured, op.operationId, op.payloadDigest, sha(refused.canonicalBytes)))
        assertFalse(status.dismissRejected(captured, op.operationId, op.payloadDigest, sha(refused.canonicalBytes)))
        val entry = queue.entry(captured, op.operationId)!!
        assertTrue(entry.dismissedRejected); assertArrayEquals(ack.canonicalBytes, entry.acknowledgement!!.canonicalBytes)
        assertArrayEquals(refused.canonicalBytes, entry.resolution!!.canonicalBytes)
        val page = status.page(captured, EnterpriseSyncMode.HISTORY)
        val row = page.items.single { it.identity.operationId == op.operationId }
        assertEquals(EnterpriseSyncStatus.REJECTED, row.status); assertFalse(row.pending)
        assertEquals(SyncResult.ACCEPTED_FOR_REVIEW, row.receipt!!.result)
        assertEquals("Synthetic private refusal 😀", row.resolution!!.reason)
        assertEquals(1, page.totals.reviewCount); assertEquals(1, page.totals.rejectedCount); assertEquals(1, page.totals.hiddenRejectedCount)
        assertFalse(row.toString().contains("Synthetic private refusal"))
    }

    @Test fun acceptedResolutionDetailKeepsOriginalReceiptAndAddsDecisionHistory(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val op = operation(operationId = id(100)); val ack = receipt(op)
        seed(f, op, ack)
        val queue = ProtectedEnterpriseQueue(f.store); val status = ProtectedEnterpriseSyncStatusStore(f.store); val captured = attempt(f)
        assertTrue(queue.resolve(captured, op, ack, decision(op)))
        val detail = status.detail(captured, op.operationId, op.payloadDigest)
        assertEquals(EnterpriseSyncStatus.ACCEPTED, detail.item.status)
        assertEquals(ack.receivedAt, detail.item.receipt!!.receivedAt)
        assertEquals(listOf(EnterpriseSyncHistoryKind.DURABLE_SAVE, EnterpriseSyncHistoryKind.SERVER_RECEIPT, EnterpriseSyncHistoryKind.REVIEW_RESOLUTION),
            detail.history.map { it.kind })
        assertEquals(java.time.Instant.parse("2026-11-13T12:05:00.123456789Z"), detail.item.resolution!!.resolvedAt)
        assertEquals(EnterpriseReviewOutcome.ACCEPTED, detail.item.resolution!!.outcome)
        val pending = status.page(captured, EnterpriseSyncMode.PENDING)
        assertTrue(pending.items.isEmpty()); assertEquals(1, pending.totals.acceptedCount); assertEquals(0, pending.totals.reviewCount)
    }

    @Test fun concurrentResolversPublishExactlyOneTerminalDecision(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val op = operation(operationId = id(100)); val ack = receipt(op)
        seed(f, op, ack)
        val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f); val before = requireNotNull(head(f))
        val pool = Executors.newFixedThreadPool(8)
        val ready = CountDownLatch(8); val start = CountDownLatch(1)
        try {
            val jobs = (1..8).map { index ->
                pool.submit(Callable<String> {
                    ready.countDown(); check(start.await(5, TimeUnit.SECONDS))
                    try {
                        runBlocking {
                            if (index % 2 == 0) { if (queue.resolve(captured, op, ack, decision(op))) "written" else "replay" }
                            else { if (queue.reviewRetry(captured, op, ack, QueueRetryState(), QueueRetryState(1, f.now, 30_000))) "retry" else "stale" }
                        }
                    } catch (_: EnterprisePersistenceUnavailable) { "unavailable" }
                })
            }
            assertTrue(ready.await(5, TimeUnit.SECONDS)); start.countDown()
            val results = jobs.map { it.get(10, TimeUnit.SECONDS) }
            assertEquals(1, results.count { it == "written" }, results.toString())
            assertEquals(3, results.count { it == "replay" }, results.toString())
            assertTrue(results.count { it == "retry" } <= 1, results.toString())
            assertFalse("unavailable" in results)
        } finally { pool.shutdownNow() }
        val entry = queue.entry(captured, op.operationId)!!
        assertArrayEquals(decision(op).canonicalBytes, entry.resolution!!.canonicalBytes)
        assertTrue(entry.reviewRetry.initial, "Resolution must clear review retry metadata")
        assertTrue(head(f)!! <= before + 2)
    }

    @Test fun unresolvedListingRefusesStaleSourceAndForeignEnrollment(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val op = operation(operationId = id(100)); seed(f, op, receipt(op))
        val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f)
        assertEquals(1, queue.unresolvedReviews(captured).size)
        f.identity = EnterpriseLocalIdentity(B, VaultFixture.ORIGIN, B)
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { queue.unresolvedReviews(captured) } }
    }
}
