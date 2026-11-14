package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.application.enterprise.sync.*
import com.anpfuel.data.enterprise.ProtectedQueueFixture.account
import com.anpfuel.data.enterprise.ProtectedQueueFixture.attempt
import com.anpfuel.data.enterprise.ProtectedQueueFixture.id
import com.anpfuel.data.enterprise.ProtectedQueueFixture.operation
import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.C
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseSyncStatusStore
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.sync.*
import java.io.ByteArrayOutputStream
import java.io.DataOutputStream
import java.nio.ByteBuffer
import java.nio.file.Path
import java.security.MessageDigest
import java.util.concurrent.Callable
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.Future
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

class ProtectedEnterpriseSyncStatusStoreTest {
    @TempDir lateinit var root: Path
    private fun generations(f: VaultFixture) = EnterpriseVaultGenerations(f.files, f.keys)
    private fun head(f: VaultFixture, p: EnterprisePartition = partition()) = generations(f).head(EnterpriseVaultCodec.token(p))
    private fun sha(bytes: ByteArray) = MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it.toInt() and 255) }
    private fun receiptDigest(ack: OperationAcknowledgement) = "sha256:" + sha(ack.canonicalBytes)
    private fun receipt(op: EnterpriseOperation, result: String = "REJECTED"): OperationAcknowledgement {
        val code = when (result) { "ACCEPTED" -> "null"; "ACCEPTED_FOR_REVIEW" -> "\"ORPHAN_REFERENCE\""; else -> "\"MALFORMED\"" }
        val revision = if (result == "ACCEPTED") "1" else "null"
        val review = if (result == "ACCEPTED_FOR_REVIEW") "\"${id(800)}\"" else "null"
        return OperationAcknowledgement.parse("""{"schema_version":1,"operation_id":"${op.operationId}","payload_digest":"${op.payloadDigest}","result":"$result","code":$code,"entity_revision":$revision,"received_at":"2026-11-13T12:00:01Z","review_item_id":$review,"warnings":[]}""".toByteArray(), op)
    }
    private fun attachments(op: EnterpriseOperation, ids: List<String>): EnterpriseOperation {
        val fields = (CanonicalJson.parse(op.canonicalBytes) as SyncJson.Object).fields.toMutableMap()
        fields.remove("payload_digest")
        fields["attachments"] = SyncJson.Array(ids.map { attachment -> SyncJson.Object(mapOf(
            "id" to SyncJson.Text(attachment), "sha256" to SyncJson.Text(sha(byteArrayOf(1, 2, 3))),
            "bytes" to SyncJson.Integer("3"), "mime" to SyncJson.Text("image/jpeg"))) })
        return EnterpriseOperation.seal(CanonicalJson.encode(SyncJson.Object(fields)), op.binding)
    }
    private suspend fun seed(f: VaultFixture, op: EnterpriseOperation, ack: OperationAcknowledgement? = null) {
        val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f, EnterprisePartition.of(op.binding.ownerAccountId, op.binding.apiOrigin, op.binding.organizationId))
        queue.save(captured, op) { it.put(EnterpriseLocalRecord(captured.partition, op.operationId, ("SYNTHETIC-PROJECTION-" + op.operationId).toByteArray())) }
        if (ack != null) queue.acknowledge(captured, op, ack) { }
    }
    /** Historical accepted queue wire. Independent of the new presentation encoder. */
    private fun frame(op: EnterpriseOperation, ack: OperationAcknowledgement?, version: Int = 1,
        dismissed: Boolean = false, trailing: Boolean = false, retry: QueueRetryState = QueueRetryState()): ByteArray = ByteArrayOutputStream().also { bytes ->
        DataOutputStream(bytes).use { out ->
            out.writeInt(0x51554555); out.writeInt(version)
            val canonical = op.canonicalBytes; out.writeInt(canonical.size); out.write(canonical)
            val receipt = ack?.canonicalBytes; out.writeInt(receipt?.size ?: -1); if (receipt != null) out.write(receipt)
            out.writeLong(retry.attempts); out.writeBoolean(retry.clock != null)
            retry.clock?.let { out.writeUTF(it.boot); out.writeLong(it.elapsedMillis); out.writeLong(it.wallMillis) }
            out.writeLong(retry.delayMillis)
            if (version == 2) out.writeBoolean(dismissed)
            if (trailing) out.writeByte(1)
        }
    }.toByteArray()
    private fun replaceFrame(f: VaultFixture, op: EnterpriseOperation, bytes: ByteArray) {
        val snapshot = generations(f).load(partition())!!
        snapshot.entries["QUEUE:" + op.operationId] = VaultEntry(VaultKind.QUEUE, op.operationId, "", bytes)
        generations(f).commit(snapshot)
    }

    @Test fun pagesIncludeFinalFactsAndFullSnapshotTotalsWithoutExposingPayloads(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val saved = operation(operationId = id(100))
        val accepted = operation(operationId = id(101))
        val review = operation(operationId = id(102))
        val rejected = operation(operationId = id(103))
        val withFile = attachments(operation(operationId = id(104)), listOf(id(600)))
        seed(f, saved); seed(f, accepted, receipt(accepted, "ACCEPTED")); seed(f, review, receipt(review, "ACCEPTED_FOR_REVIEW"))
        seed(f, rejected, receipt(rejected)); seed(f, withFile, receipt(withFile, "ACCEPTED"))
        f.store.enqueue(EnterprisePendingOperation(partition(), id(900), A, byteArrayOf(9)))
        val status = ProtectedEnterpriseSyncStatusStore(f.store); val captured = attempt(f); val before = head(f)
        val first = status.page(captured, EnterpriseSyncMode.PENDING, limit = 2)
        assertEquals(2, first.items.size); assertNotNull(first.next)
        assertEquals(5, first.totals.totalCount); assertEquals(4, first.totals.pendingCount)
        assertEquals(1, first.totals.savedCount); assertEquals(1, first.totals.reviewCount)
        assertEquals(1, first.totals.rejectedCount); assertEquals(2, first.totals.acceptedCount)
        assertEquals(1L, first.totals.declaredAttachmentCount); assertEquals(1L, first.totals.unconfirmedAttachmentCount)
        assertEquals(1, first.totals.storage.legacyCount)
        assertEquals(receipt(accepted, "ACCEPTED").receivedAt, first.totals.latestConfirmedReceiptAt)
        assertEquals(saved.declaration.occurredAt, first.totals.oldestPendingDeclaredAt)
        val second = status.page(attempt(f), EnterpriseSyncMode.PENDING, first.next, 2)
        assertEquals(2, second.items.size); assertNull(second.next); assertEquals(first.generation, second.generation)
        assertEquals(setOf(saved.operationId, review.operationId, rejected.operationId, withFile.operationId),
            (first.items + second.items).map { it.identity.operationId }.toSet())
        val history = status.page(captured, EnterpriseSyncMode.HISTORY)
        assertEquals(5, history.items.size); assertEquals(before, head(f), "Reads do not rewrite or upgrade framing")
        for (item in history.items) {
            assertFalse(item.toString().contains("SYNTHETIC-QUEUE"))
            assertFalse(item.javaClass.declaredFields.any { it.type == ByteArray::class.java || it.name in setOf("payload", "operation", "acknowledgement") })
        }
    }

    @Test fun detailChecksActualProtectedAttachmentBytesWithoutClaimingUpload(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val ids = listOf(id(600), id(601), id(602), id(603)); val op = attachments(operation(), ids)
        seed(f, op, receipt(op, "ACCEPTED"))
        f.store.putPrivateFile(partition(), ids[0], byteArrayOf(1, 2, 3))
        f.store.putPrivateFile(partition(), ids[1], byteArrayOf(9))
        f.store.putPrivateFile(partition(), ids[3], byteArrayOf(1, 2, 3), cache = true)
        val status = ProtectedEnterpriseSyncStatusStore(f.store); val detail = status.detail(attempt(f), op.operationId, op.payloadDigest)
        assertEquals(listOf(EnterpriseAttachmentEvidence.MATCHING, EnterpriseAttachmentEvidence.MISMATCH,
            EnterpriseAttachmentEvidence.MISSING, EnterpriseAttachmentEvidence.MISSING), detail.attachments.map { it.localEvidence })
        assertTrue(detail.attachments.none { it.uploadConfirmed })
        assertEquals(EnterpriseSyncStatus.ACCEPTED, detail.item.status)
        assertTrue(detail.item.pending, "An accepted operation with unconfirmed attachments remains in pending presentation")
        assertEquals(EnterpriseSyncHistoryKind.DURABLE_SAVE, detail.history.first().kind)
        assertNull(detail.history.first().time); assertEquals(EnterpriseSyncTimeSource.UNKNOWN, detail.history.first().source)
        assertFalse(detail.history.any { it.time == op.declaration.occurredAt && it.source == EnterpriseSyncTimeSource.DEVICE })
        assertFalse(detail.javaClass.declaredFields.any { it.type == ByteArray::class.java })
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { status.detail(attempt(f), op.operationId, "sha256:" + "0".repeat(64)) } }
    }

    @Test fun v1RefusalDismissalUpgradesOnlyVisibilityAndSurvivesRecreation(): Unit = runBlocking {
        val directory = root.resolve("vault"); val f = VaultFixture(directory); f.confirm(partition())
        val op = attachments(operation(), listOf(id(600))); val ack = receipt(op)
        seed(f, op)
        val queue = ProtectedEnterpriseQueue(f.store); val retainedRetry = QueueRetryState(3, f.now, 90_000)
        assertTrue(queue.retry(attempt(f), op, QueueRetryState(), retainedRetry))
        queue.acknowledge(attempt(f), op, ack) { }
        f.store.putPrivateFile(partition(), id(600), byteArrayOf(1, 2, 3))
        replaceFrame(f, op, frame(op, ack, retry = retainedRetry))
        val status = ProtectedEnterpriseSyncStatusStore(f.store); val captured = attempt(f)
        val original = generations(f).load(partition())!!.entries.getValue("QUEUE:" + op.operationId).bytes.copyOf()
        assertEquals(1, ByteBuffer.wrap(original).getInt(4))
        assertFalse(status.detail(captured, op.operationId, op.payloadDigest).item.dismissedRejected)
        val before = requireNotNull(head(f)); val record = f.store.record(partition(), op.operationId)!!.envelope
        assertTrue(status.dismissRejected(captured, op.operationId, op.payloadDigest, receiptDigest(ack)))
        assertEquals(before + 1, head(f))
        assertFalse(status.dismissRejected(captured, op.operationId, op.payloadDigest, receiptDigest(ack)))
        assertEquals(before + 1, head(f), "Repeated dismissal does not publish a new generation")
        val reopened = VaultFixture(directory, f.keys); val restored = ProtectedEnterpriseSyncStatusStore(reopened.store); val current = attempt(reopened)
        val pending = restored.page(current, EnterpriseSyncMode.PENDING)
        assertTrue(pending.items.isEmpty()); assertEquals(0, pending.totals.pendingCount)
        assertEquals(1, pending.totals.hiddenRejectedCount); assertEquals(1L, pending.totals.declaredAttachmentCount)
        assertEquals(0L, pending.totals.unconfirmedAttachmentCount)
        val retained = restored.detail(current, op.operationId, op.payloadDigest)
        assertTrue(retained.item.dismissedRejected); assertEquals(receiptDigest(ack), retained.item.receipt!!.receiptDigest)
        assertEquals(1, restored.page(current, EnterpriseSyncMode.HISTORY).items.size)
        val queueEntry = ProtectedEnterpriseQueue(reopened.store).entry(current, op.operationId)!!
        assertArrayEquals(op.canonicalBytes, queueEntry.operation.canonicalBytes)
        assertArrayEquals(ack.canonicalBytes, queueEntry.acknowledgement!!.canonicalBytes)
        assertEquals(retainedRetry.attempts, queueEntry.retry.attempts); assertEquals(retainedRetry.clock, queueEntry.retry.clock)
        assertEquals(retainedRetry.delayMillis, queueEntry.retry.delayMillis)
        val afterDismissal = head(reopened)
        val existing = ProtectedEnterpriseQueue(reopened.store)
        assertFalse(existing.save(current, op) { fail("Dismissed operation was projected again") })
        assertFalse(existing.acknowledge(current, op, ack) { fail("Dismissed receipt was projected again") })
        assertFalse(existing.retry(current, op, retainedRetry, QueueRetryState(4, reopened.now, 120_000)))
        assertEquals(afterDismissal, head(reopened)); assertTrue(restored.detail(current, op.operationId, op.payloadDigest).item.dismissedRejected)
        assertArrayEquals(record, reopened.store.record(partition(), op.operationId)!!.envelope)
        assertArrayEquals(byteArrayOf(1, 2, 3), reopened.store.privateFile(partition(), id(600)))
        assertEquals(2, ByteBuffer.wrap(generations(reopened).load(partition())!!.entries.getValue("QUEUE:" + op.operationId).bytes).getInt(4))
    }

    @Test fun cursorBindsModeTupleAndTrustedGenerationWithoutMixingPages(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition()); f.confirm(partition(org = B))
        val ops = (100..102).map { operation(operationId = id(it)) }; for (op in ops) seed(f, op)
        val status = ProtectedEnterpriseSyncStatusStore(f.store); val captured = attempt(f)
        val first = status.page(captured, EnterpriseSyncMode.PENDING, limit = 1); val cursor = first.next!!
        assertFalse(cursor.toString().contains(A)); assertFalse(cursor.toString().contains(partition().origin))
        assertThrows(EnterpriseSyncPageChanged::class.java) { runBlocking { status.page(captured, EnterpriseSyncMode.HISTORY, cursor, 1) } }
        assertThrows(EnterpriseSyncPageChanged::class.java) { runBlocking { status.page(attempt(f, partition(org = B)), EnterpriseSyncMode.PENDING, cursor, 1) } }
        assertThrows(EnterpriseSyncPageChanged::class.java) { runBlocking { status.page(captured, EnterpriseSyncMode.PENDING, object : EnterpriseSyncCursor { }, 1) } }
        val queue = ProtectedEnterpriseQueue(f.store)
        assertTrue(queue.retry(captured, ops[0], QueueRetryState(), QueueRetryState(1, f.now, 30_000)))
        assertThrows(EnterpriseSyncPageChanged::class.java) { runBlocking { status.page(captured, EnterpriseSyncMode.PENDING, cursor, 1) } }
        assertTrue(status.page(captured, EnterpriseSyncMode.PENDING, limit = 1).generation > first.generation)
        assertThrows(IllegalArgumentException::class.java) { runBlocking { status.page(captured, EnterpriseSyncMode.PENDING, limit = 0) } }
        assertThrows(IllegalArgumentException::class.java) { runBlocking { status.page(captured, EnterpriseSyncMode.PENDING, limit = 51) } }
    }

    @Test fun wrongIntentReceiptAndNonRefusedStatesNeverHideOrMutateAnyProjection(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val status = ProtectedEnterpriseSyncStatusStore(f.store); val captured = attempt(f)
        for ((index, result) in listOf("PENDING", "ACCEPTED", "ACCEPTED_FOR_REVIEW", "REJECTED").withIndex()) {
            val op = operation(operationId = id(100 + index)); val ack = if (result == "PENDING") null else receipt(op, result)
            seed(f, op, ack); val before = head(f); val record = f.store.record(partition(), op.operationId)!!.envelope
            assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking {
                status.dismissRejected(captured, op.operationId, "sha256:" + "0".repeat(64), ack?.let(::receiptDigest) ?: "sha256:" + "1".repeat(64))
            } }
            assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking {
                status.dismissRejected(captured, op.operationId, op.payloadDigest, "sha256:" + "0".repeat(64))
            } }
            if (result != "REJECTED") assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking {
                status.dismissRejected(captured, op.operationId, op.payloadDigest, ack?.let(::receiptDigest) ?: "sha256:" + "1".repeat(64))
            } }
            assertEquals(before, head(f)); assertArrayEquals(record, f.store.record(partition(), op.operationId)!!.envelope)
        }
    }

    @Test fun everyDismissalPublicationFaultRetainsTheVisibleV1RefusalUntilRecovery(): Unit = runBlocking {
        for (stage in listOf("data", "descriptor", "head")) {
            var armed = false; val keys = TestVaultKeys()
            val f = VaultFixture(root.resolve(stage), keys) { real -> object : EnterpriseVaultFiles by real {
                override fun replaceGeneration(token: String, generation: Long, descriptor: Boolean, ciphertext: ByteArray) {
                    if (armed && stage == if (descriptor) "descriptor" else "data") error("synthetic visibility publication failure")
                    real.replaceGeneration(token, generation, descriptor, ciphertext)
                }
            } }
            f.confirm(partition()); val op = operation(); val ack = receipt(op); seed(f, op, ack); replaceFrame(f, op, frame(op, ack))
            val status = ProtectedEnterpriseSyncStatusStore(f.store); val captured = attempt(f); val before = head(f)
            keys.afterCreate = { alias -> if (armed && stage == "head" && alias.endsWith("_m")) keys.failCreate = true }
            armed = true
            assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { status.dismissRejected(captured, op.operationId, op.payloadDigest, receiptDigest(ack)) } }
            armed = false; keys.failCreate = false; keys.afterCreate = null
            assertEquals(before, head(f)); assertFalse(status.detail(captured, op.operationId, op.payloadDigest).item.dismissedRejected)
            assertArrayEquals(ack.canonicalBytes, ProtectedEnterpriseQueue(f.store).entry(captured, op.operationId)!!.acknowledgement!!.canonicalBytes)
            assertTrue(status.dismissRejected(captured, op.operationId, op.payloadDigest, receiptDigest(ack)))
        }
    }

    @Test fun invalidVisibilityAndUnknownOrTrailingFramesNeverBecomeSuccessfulEmptyState(): Unit = runBlocking {
        for (variant in listOf("pending_hidden", "accepted_hidden", "review_hidden", "future", "v1_trailing", "v2_trailing", "v2_bad_boolean", "v2_missing_boolean")) {
            val f = VaultFixture(root.resolve(variant)); f.confirm(partition()); val op = operation()
            val ack = when (variant) { "pending_hidden" -> null; "accepted_hidden" -> receipt(op, "ACCEPTED"); "review_hidden" -> receipt(op, "ACCEPTED_FOR_REVIEW"); else -> receipt(op) }
            seed(f, op, ack)
            val bytes = when (variant) {
                "future" -> frame(op, ack, version = 3)
                "v1_trailing" -> frame(op, ack, trailing = true)
                "v2_trailing" -> frame(op, ack, version = 2, dismissed = true, trailing = true)
                "v2_bad_boolean" -> frame(op, ack, version = 2, dismissed = true).also { it[it.lastIndex] = 2 }
                "v2_missing_boolean" -> frame(op, ack, version = 2, dismissed = true).dropLast(1).toByteArray()
                else -> frame(op, ack, version = 2, dismissed = true)
            }
            replaceFrame(f, op, bytes)
            val status = ProtectedEnterpriseSyncStatusStore(f.store); val captured = attempt(f)
            val before = head(f); val aliases = f.keys.values.keys.toSet()
            assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { status.page(captured, EnterpriseSyncMode.PENDING) } }
            assertEquals(before, head(f)); assertEquals(aliases, f.keys.values.keys)
            assertArrayEquals(bytes, generations(f).load(partition())!!.entries.getValue("QUEUE:" + op.operationId).bytes)
            assertFalse(f.erasure.cleanupRequired); assertFalse(f.files.denied(EnterpriseVaultCodec.token(partition())))
        }
    }

    @Test fun actualSourceChangesDuringReadAndAlwaysTrueForgedLeasesCannotExposeStatus(): Unit = runBlocking {
        var armed = false; var change: () -> Unit = { }
        val f = VaultFixture(root.resolve("vault")) { real -> object : EnterpriseVaultFiles by real {
            override fun readGeneration(token: String, generation: Long, descriptor: Boolean): ByteArray? {
                val bytes = real.readGeneration(token, generation, descriptor)
                if (armed && !descriptor) { armed = false; change() }
                return bytes
            }
        } }
        f.confirm(partition()); val op = operation(); val ack = receipt(op); seed(f, op, ack)
        val status = ProtectedEnterpriseSyncStatusStore(f.store); val captured = attempt(f); val before = head(f)
        for (wrong in listOf(captured.account.identity.copy(account = B), captured.account.identity.copy(family = B))) {
            val forged = EnterpriseQueueAccountLease.capture(wrong, f.fence.epoch, f.fence.backgroundEpoch, id(901)) { true }
            val target = if (wrong.account == B) captured.enrollment.copy(target = EnterpriseDeviceTarget(partition(owner = B), B)) else captured.enrollment
            val attempt = forged.forEnrollment(target)
            assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { status.page(attempt, EnterpriseSyncMode.HISTORY) } }
            assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { status.detail(attempt, op.operationId, op.payloadDigest) } }
            assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { status.dismissRejected(attempt, op.operationId, op.payloadDigest, receiptDigest(ack)) } }
        }
        change = { f.fence.markBackground() }; armed = true
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { status.page(captured, EnterpriseSyncMode.HISTORY) } }
        assertEquals(before, head(f)); assertFalse(status.detail(attempt(f), op.operationId, op.payloadDigest).item.dismissedRejected)
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { status.dismissRejected(captured, op.operationId, op.payloadDigest, receiptDigest(ack)) } }
    }

    @Test fun fullByteBudgetCannotReportDismissalBeforeItsOneByteUpgradeIsDurable(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition()); val op = operation(); val ack = receipt(op)
        seed(f, op, ack); val legacy = frame(op, ack)
        val snapshot = generations(f).load(partition())!!
        snapshot.entries["QUEUE:" + op.operationId] = VaultEntry(VaultKind.QUEUE, op.operationId, "", legacy)
        val record = snapshot.entries.getValue("RECORD:" + op.operationId).bytes.copyOf()
        val overhead = 2048L + snapshot.entries.values.sumOf { it.bytes.size.toLong() + 512 } + 512
        val fileSize = EnterpriseVaultCodec.MAX_PLAIN - overhead.toInt()
        snapshot.entries["ATTACHMENT:$C"] = VaultEntry(VaultKind.ATTACHMENT, C, "", ByteArray(fileSize))
        generations(f).commit(snapshot)
        val status = ProtectedEnterpriseSyncStatusStore(f.store); val captured = attempt(f); val before = head(f)
        assertEquals(EnterpriseVaultCodec.MAX_PLAIN.toLong(), status.page(captured, EnterpriseSyncMode.PENDING).totals.storage.totalBytes)
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { status.dismissRejected(captured, op.operationId, op.payloadDigest, receiptDigest(ack)) } }
        assertEquals(before, head(f)); assertFalse(status.detail(captured, op.operationId, op.payloadDigest).item.dismissedRejected)
        assertArrayEquals(ack.canonicalBytes, ProtectedEnterpriseQueue(f.store).entry(captured, op.operationId)!!.acknowledgement!!.canonicalBytes)
        assertArrayEquals(record, f.store.record(partition(), op.operationId)!!.envelope)
        assertEquals(fileSize, f.store.privateFile(partition(), C)!!.size)
    }

    @Test fun parallelDismissalsAndProjectionWritePublishOneHideWithoutMixingReadsOrOtherScopes(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition()); f.confirm(partition(org = B))
        val refused = attachments(operation(operationId = id(100)), listOf(id(600))); val ack = receipt(refused)
        val pending = operation(operationId = id(101), value = "SYNTHETIC-NEWER-PENDING-EDIT")
        val other = operation(p = partition(org = B), operationId = id(102), value = "SYNTHETIC-OTHER-ORG")
        seed(f, refused, ack); seed(f, pending); seed(f, other)
        f.store.put(EnterpriseLocalRecord(partition(), A, refused.canonicalBytes))
        f.store.putPrivateFile(partition(), id(600), byteArrayOf(1, 2, 3))
        f.store.putPrivateFile(partition(org = B), id(601), byteArrayOf(8, 9))
        val status = ProtectedEnterpriseSyncStatusStore(f.store); val captured = attempt(f)
        val before = requireNotNull(head(f)); val otherHead = requireNotNull(head(f, partition(org = B)))
        val otherToken = EnterpriseVaultCodec.token(partition(org = B))
        val otherPayload = f.files.readGeneration(otherToken, otherHead, false)!!.copyOf()
        val otherMetadata = f.files.readGeneration(otherToken, otherHead, true)!!.copyOf()
        val threads = AtomicInteger()
        val pool = Executors.newFixedThreadPool(10) { work ->
            Thread(work, "synthetic-sync-visibility-" + threads.incrementAndGet()).apply { isDaemon = true }
        }
        val ready = CountDownLatch(10); val start = CountDownLatch(1)
        val jobs = mutableListOf<Future<*>>()
        val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(5)
        fun remaining() = (deadline - System.nanoTime()).coerceAtLeast(0)
        fun enter() {
            ready.countDown()
            check(start.await(remaining(), TimeUnit.NANOSECONDS)) { "Synthetic visibility gate timed out" }
        }
        try {
            val dismissals = (1..8).map {
                pool.submit(Callable<Boolean> {
                    enter()
                    runBlocking { status.dismissRejected(captured, refused.operationId, refused.payloadDigest, receiptDigest(ack)) }
                }).also { jobs += it }
            }
            val writer = pool.submit(Callable<Unit> {
                enter()
                runBlocking { f.store.put(EnterpriseLocalRecord(partition(), A, pending.canonicalBytes)) }
            }).also { jobs += it }
            val reader = pool.submit(Callable<Unit> {
                enter()
                runBlocking {
                    repeat(4) {
                        val page = status.page(captured, EnterpriseSyncMode.HISTORY)
                        assertEquals(2, page.items.size)
                        assertEquals(page.items.count { row -> row.dismissedRejected }, page.totals.hiddenRejectedCount)
                        assertEquals(page.items.count { row -> row.pending }, page.totals.pendingCount)
                        val hidden = page.items.single { row -> row.identity.operationId == refused.operationId }.dismissedRejected
                        assertEquals(if (hidden) 0L else 1L, page.totals.unconfirmedAttachmentCount)
                    }
                }
            }).also { jobs += it }
            assertTrue(ready.await(remaining(), TimeUnit.NANOSECONDS), "Parallel visibility callers did not reach their common gate")
            start.countDown()
            val results = dismissals.map { it.get(remaining(), TimeUnit.NANOSECONDS) }
            writer.get(remaining(), TimeUnit.NANOSECONDS); reader.get(remaining(), TimeUnit.NANOSECONDS)
            assertEquals(1, results.count { it }); assertEquals(7, results.count { !it })
        } finally {
            start.countDown()
            jobs.forEach { it.cancel(true) }
            pool.shutdownNow()
            pool.awaitTermination(remaining(), TimeUnit.NANOSECONDS)
        }
        val published = requireNotNull(head(f))
        assertEquals(before + 2, published, "Exactly one visibility commit and one owning projection write are published")
        assertFalse(status.dismissRejected(captured, refused.operationId, refused.payloadDigest, receiptDigest(ack)))
        val queue = ProtectedEnterpriseQueue(f.store)
        assertFalse(queue.retry(captured, refused, QueueRetryState(), QueueRetryState(1, f.now, 30_000)))
        assertEquals(published, head(f), "No-op visibility and retry calls cannot create another head")
        assertArrayEquals(pending.canonicalBytes, f.store.record(partition(), A)!!.envelope)
        assertArrayEquals(refused.canonicalBytes, queue.entry(captured, refused.operationId)!!.operation.canonicalBytes)
        assertArrayEquals(ack.canonicalBytes, queue.entry(captured, refused.operationId)!!.acknowledgement!!.canonicalBytes)
        assertTrue(queue.entry(captured, refused.operationId)!!.dismissedRejected)
        assertArrayEquals(pending.canonicalBytes, queue.pending(captured).single().operation.canonicalBytes)
        assertFalse(queue.entry(captured, pending.operationId)!!.dismissedRejected)
        assertArrayEquals(byteArrayOf(1, 2, 3), f.store.privateFile(partition(), id(600)))
        assertEquals(otherHead, head(f, partition(org = B)))
        assertArrayEquals(otherPayload, f.files.readGeneration(otherToken, otherHead, false))
        assertArrayEquals(otherMetadata, f.files.readGeneration(otherToken, otherHead, true))
        assertArrayEquals(other.canonicalBytes, queue.pending(attempt(f, partition(org = B))).single().operation.canonicalBytes)
        assertArrayEquals(byteArrayOf(8, 9), f.store.privateFile(partition(org = B), id(601)))
    }
}
