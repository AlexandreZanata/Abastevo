package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.C
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.sync.*
import java.nio.file.Files
import java.nio.file.Path
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

class ProtectedEnterpriseQueueTest {
    @TempDir lateinit var root: Path
    private fun id(value: Int) = "7b0e2a54-0000-4000-8000-" + value.toString().padStart(12, '0')
    private fun operation(p: EnterprisePartition = partition(), value: String = "SYNTHETIC-PRIVATE-QUEUE",
        operationId: String = id(100), membership: String = B, enrollment: String = C): EnterpriseOperation =
        EnterpriseOperation.seal("""{"schema_version":1,"operation_id":"$operationId","owner_account_id":"${p.ownerAccount}","api_origin":"${p.origin}","org_id":"${p.organizationId}","membership_id":"$membership","enrollment_id":"$enrollment","type":"validation.record","operation_schema":1,"entity":{"type":"record","id":"$A","expected_revision":null},"occurred_at":"2026-11-13T12:00:00Z","entry_zone":"UTC","device_offset_seconds":0,"depends_on":[],"soft_references":[],"attachments":[],"payload":{"value":"$value"}}""".toByteArray(),
            SyncBinding.of(p.ownerAccount, p.origin, p.organizationId))

    private fun ack(op: EnterpriseOperation, revision: Long = 1): OperationAcknowledgement =
        OperationAcknowledgement.parse("""{"schema_version":1,"operation_id":"${op.operationId}","payload_digest":"${op.payloadDigest}","result":"ACCEPTED","code":null,"entity_revision":$revision,"received_at":"2026-11-13T12:00:01Z","review_item_id":null,"warnings":[]}""".toByteArray(), op)

    private fun account(f: VaultFixture): EnterpriseQueueAccountLease {
        val identity = requireNotNull(f.identity)
        val epoch = f.fence.epoch
        val background = f.fence.backgroundEpoch
        return EnterpriseQueueAccountLease.capture(identity, epoch, background, id(900)) {
            f.identity == identity && f.fence.epoch == epoch && f.fence.backgroundEpoch == background
        }
    }
    private suspend fun attempt(f: VaultFixture, p: EnterprisePartition = partition()): EnterpriseQueueAttempt {
        val account = account(f)
        return account.forEnrollment(f.store.protectedEnrollment(account, p))
    }
    private fun head(f: VaultFixture, p: EnterprisePartition = partition()) =
        EnterpriseVaultGenerations(f.files, f.keys).head(EnterpriseVaultCodec.token(p))

    @Test fun localSaveAndFinalReceiptCommitOnceWithTheirRealProjectionAcrossReopen(): Unit = runBlocking {
        val directory = root.resolve("vault")
        val f = VaultFixture(directory)
        f.confirm(partition())
        val queue = ProtectedEnterpriseQueue(f.store)
        val captured = attempt(f)
        val operation = operation()
        var saves = 0
        var receipts = 0
        val save: (EnterpriseQueueTransaction) -> Unit = { tx ->
            saves++
            assertNull(tx.record(A))
            tx.put(EnterpriseLocalRecord(partition(), A, operation.canonicalBytes))
        }
        assertTrue(queue.save(captured, operation, save))
        assertFalse(queue.save(captured, operation, save))
        assertEquals(1, saves)
        assertArrayEquals(operation.canonicalBytes, f.store.record(partition(), A)!!.envelope)
        assertArrayEquals(operation.canonicalBytes, queue.pending(captured).single().operation.canonicalBytes)
        val afterSave = head(f)
        val reopened = VaultFixture(directory, f.keys)
        val recoveredQueue = ProtectedEnterpriseQueue(reopened.store)
        val recovered = attempt(reopened)
        val receipt = ack(operation)
        val project: (EnterpriseQueueTransaction) -> Unit = { tx ->
            receipts++
            assertArrayEquals(operation.canonicalBytes, tx.record(A)!!.envelope)
            tx.put(EnterpriseLocalRecord(partition(), A, receipt.canonicalBytes))
        }
        assertTrue(recoveredQueue.acknowledge(recovered, operation, receipt, project))
        assertFalse(recoveredQueue.acknowledge(recovered, operation, receipt, project))
        assertEquals(1, receipts)
        assertTrue(recoveredQueue.pending(recovered).isEmpty())
        assertArrayEquals(receipt.canonicalBytes, recoveredQueue.entry(recovered, operation.operationId)!!.acknowledgement!!.canonicalBytes)
        assertArrayEquals(receipt.canonicalBytes, reopened.store.record(partition(), A)!!.envelope)
        assertEquals(afterSave!! + 1, head(reopened), "A replay does not prepare another generation")
        assertEquals(2, recoveredQueue.stats(recovered).totalEntries, "Final receipt remains retained")
    }

    @Test fun changedIntentOrFinalReceiptNeverReplacesTheOriginal(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f); val operation = operation()
        queue.save(captured, operation) { it.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(7))) }
        assertThrows(EnterpriseOperationReused::class.java) { runBlocking {
            queue.save(captured, operation(value = "DIFFERENT")) { fail("Changed intent projected") }
        } }
        queue.acknowledge(captured, operation, ack(operation)) { }
        val trusted = head(f)
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking {
            queue.acknowledge(captured, operation, ack(operation, 2)) { fail("Changed final receipt projected") }
        } }
        assertEquals(trusted, head(f))
        assertArrayEquals(byteArrayOf(7), f.store.record(partition(), A)!!.envelope)
    }

    @Test fun projectorFailureAndEveryGenerationPreparationFailureKeepBothOldValues(): Unit = runBlocking {
        for (stage in listOf("project", "data", "descriptor", "head")) {
            var armed = false
            val keys = TestVaultKeys()
            val f = VaultFixture(root.resolve(stage), keys) { real -> object : EnterpriseVaultFiles by real {
                override fun replaceGeneration(token: String, generation: Long, descriptor: Boolean, ciphertext: ByteArray) {
                    if (armed && stage == if (descriptor) "descriptor" else "data") error("synthetic provider fault")
                    real.replaceGeneration(token, generation, descriptor, ciphertext)
                }
            } }
            f.confirm(partition())
            f.store.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(7)))
            val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f); val operation = operation()
            val previous = head(f)
            keys.afterCreate = { alias -> if (armed && stage == "head" && alias.endsWith("_m")) keys.failCreate = true }
            armed = true
            assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking {
                queue.save(captured, operation) { tx ->
                    tx.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(8)))
                    if (stage == "project") error("synthetic projector fault")
                }
            } }
            armed = false; keys.failCreate = false; keys.afterCreate = null
            assertEquals(previous, head(f), stage)
            assertTrue(queue.pending(captured).isEmpty(), stage)
            assertArrayEquals(byteArrayOf(7), f.store.record(partition(), A)!!.envelope, stage)
            assertTrue(queue.save(captured, operation) { it.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(8))) })
            assertEquals(1, queue.pending(captured).size)
        }
    }

    @Test fun borrowedProjectionCannotEscapeItsScopeOrOverwriteANewerEdit(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f)
        val first = operation(); val newer = operation(operationId = id(101), value = "NEWER")
        lateinit var borrowed: EnterpriseQueueTransaction
        queue.save(captured, first) { borrowed = it; it.put(EnterpriseLocalRecord(partition(), A, first.canonicalBytes)) }
        assertThrows(EnterpriseLocalDenied::class.java) { borrowed.record(A) }
        assertThrows(EnterpriseLocalDenied::class.java) { borrowed.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(9))) }
        queue.save(captured, newer) { it.put(EnterpriseLocalRecord(partition(), A, newer.canonicalBytes)) }
        queue.acknowledge(captured, first, ack(first)) { tx ->
            if (tx.record(A)!!.envelope.contentEquals(first.canonicalBytes)) tx.put(EnterpriseLocalRecord(partition(), A, ack(first).canonicalBytes))
        }
        assertArrayEquals(newer.canonicalBytes, f.store.record(partition(), A)!!.envelope)
        assertEquals(newer.operationId, queue.pending(captured).single().operation.operationId)
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking {
            queue.save(captured, operation(operationId = id(102))) { it.put(EnterpriseLocalRecord(partition(org = B), A, byteArrayOf(8))) }
        } }
    }

    @Test fun boundMemberEnrollmentAndReplacedAttemptAreIndependentlyRechecked(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f); val previous = head(f)
        for (wrong in listOf(operation(membership = A), operation(enrollment = B), operation(p = partition(org = B)))) {
            assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { queue.save(captured, wrong) { fail("Foreign operation projected") } } }
        }
        val fabricated = captured.account.forEnrollment(captured.enrollment.copy(revision = 2))
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { queue.pending(fabricated) } }
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking {
            queue.save(captured, operation()) { tx ->
                tx.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(8)))
                f.fence.markBackground()
            }
        } }
        assertEquals(previous, head(f))
        assertNull(f.store.record(partition(), A))
        assertTrue(queue.pending(attempt(f)).isEmpty())
    }

    @Test fun retryAndPartialReceiptRecoveryNeverFinalizeOrReapplyOthers(): Unit = runBlocking {
        val directory = root.resolve("vault"); val f = VaultFixture(directory); f.confirm(partition())
        val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f)
        val first = operation(); val second = operation(operationId = id(101))
        queue.save(captured, first) { }; queue.save(captured, second) { }
        val retry = QueueRetryState(Long.MAX_VALUE, f.now, Long.MAX_VALUE)
        assertTrue(queue.retry(captured, first, QueueRetryState(), retry))
        queue.acknowledge(captured, second, ack(second)) { it.put(EnterpriseLocalRecord(partition(), B, byteArrayOf(2))) }
        val committed = head(f)
        assertFalse(queue.retry(captured, second, QueueRetryState(), QueueRetryState(1, f.now, 3_600_000)))
        assertEquals(committed, head(f), "A late retry cannot rewrite a retained final receipt")
        val reopened = VaultFixture(directory, f.keys); val restored = ProtectedEnterpriseQueue(reopened.store)
        val remaining = restored.pending(attempt(reopened)).single()
        assertEquals(first.operationId, remaining.operation.operationId)
        assertEquals(Long.MAX_VALUE, remaining.retry.attempts)
        assertEquals(Long.MAX_VALUE, remaining.retry.delayMillis)
        assertEquals(f.now, remaining.retry.clock)
        assertArrayEquals(byteArrayOf(2), reopened.store.record(partition(), B)!!.envelope)
        val nonfinal = OperationAcknowledgement.parse("""{"schema_version":1,"operation_id":"${first.operationId}","payload_digest":"${first.payloadDigest}","result":"RETRY","code":"DEPENDENCY_PENDING","entity_revision":null,"received_at":null,"review_item_id":null,"warnings":[]}""".toByteArray(), first)
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking {
            restored.acknowledge(attempt(reopened), first, nonfinal) { fail("RETRY projected a final receipt") }
        } }
        assertEquals(1, restored.pending(attempt(reopened)).size)
    }

    @Test fun adversarialAlwaysTrueLeaseCannotAdoptActualEpochBackgroundOrFamily(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val queue = ProtectedEnterpriseQueue(f.store)
        val real = attempt(f)
        val capturedIdentity = real.account.identity
        val epoch = f.fence.epoch
        val background = f.fence.backgroundEpoch
        val forged = listOf(
            EnterpriseQueueAccountLease.capture(capturedIdentity, epoch + 1, background, id(910)) { true },
            EnterpriseQueueAccountLease.capture(capturedIdentity, epoch, background + 1, id(911)) { true },
            EnterpriseQueueAccountLease.capture(capturedIdentity.copy(family = B), epoch, background, id(912)) { true },
        )
        val trusted = head(f)
        for (account in forged) {
            val attempt = account.forEnrollment(real.enrollment)
            assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { queue.pending(attempt) } }
            assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { f.store.protectedEnrollment(account, partition()) } }
            assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { f.store.discover(account) } }
        }
        f.fence.markBackground()
        val old = EnterpriseQueueAccountLease.capture(capturedIdentity, epoch, background, id(913)) { true }.forEnrollment(real.enrollment)
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { queue.save(old, operation()) { fail("Old source projected") } } }
        assertEquals(trusted, head(f))
    }

    @Test fun opaqueLegacyWorkStaysUnsentAndAllOtherSurfacesSurviveUpgrade(): Unit = runBlocking {
        val directory = root.resolve("vault"); val personal = root.resolve("personal.db")
        Files.write(personal, byteArrayOf(9, 8, 7))
        val f = VaultFixture(directory); f.confirm(partition())
        val operation = operation()
        f.store.enqueue(EnterprisePendingOperation(partition(), id(400), A, operation.canonicalBytes))
        f.store.putPrivateFile(partition(), B, byteArrayOf(6))
        f.store.putPrivateFile(partition(), C, byteArrayOf(5), true)
        val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f)
        assertTrue(queue.pending(captured).isEmpty(), "Legacy bytes that resemble an operation cannot enter the typed queue")
        val beforeCollision = head(f)
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking {
            queue.save(captured, operation(operationId = id(400))) { fail("Legacy operation identity was adopted") }
        } }
        assertEquals(beforeCollision, head(f))
        assertArrayEquals(operation.canonicalBytes, f.store.pending(partition(), 50).single().envelope)
        queue.save(captured, operation) { it.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(4))) }
        val reopened = VaultFixture(directory, f.keys); val restored = ProtectedEnterpriseQueue(reopened.store)
        assertEquals(1, restored.pending(attempt(reopened)).size)
        assertEquals(1, restored.stats(attempt(reopened)).legacyCount)
        assertArrayEquals(operation.canonicalBytes, reopened.store.pending(partition(), 50).single().envelope)
        assertArrayEquals(byteArrayOf(6), reopened.store.privateFile(partition(), B))
        assertArrayEquals(byteArrayOf(5), reopened.store.privateFile(partition(), C, true))
        assertArrayEquals(byteArrayOf(9, 8, 7), Files.readAllBytes(personal))
        Files.walk(directory).use { stream -> stream.filter(Files::isRegularFile).forEach { path ->
            val bytes = Files.readAllBytes(path).toString(Charsets.ISO_8859_1)
            assertFalse(bytes.contains("SYNTHETIC-PRIVATE-QUEUE")); assertFalse(bytes.contains(A))
        } }
    }

    @Test fun discoveryUsesOnlyCurrentAccountOriginAndActualProtectedDescriptors(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault"))
        val scopes = listOf(partition(), partition(org = B), partition(owner = B), partition(origin = "https://other.example.invalid"))
        for (p in scopes) { f.identity = EnterpriseLocalIdentity(p.ownerAccount, p.origin, A); f.confirm(p) }
        f.identity = EnterpriseLocalIdentity(A, partition().origin, A)
        val captured = account(f)
        val first = f.store.discover(captured, limit = 1)
        assertEquals(1, first.enrollments.size); assertNotNull(first.nextAfterOpaque)
        val second = f.store.discover(captured, first.nextAfterOpaque, 1)
        assertEquals(1, second.enrollments.size); assertNull(second.nextAfterOpaque)
        assertEquals(setOf(A, B), (first.enrollments + second.enrollments).map { it.target.partition.organizationId }.toSet())
        assertEquals(0, first.unavailable + second.unavailable)
        assertTrue(first.nextAfterOpaque!!.matches(Regex("[0-9a-f]{64}-[0-9a-f]{64}")))
        f.identity = EnterpriseLocalIdentity(B, partition().origin, A)
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { f.store.discover(captured) } }
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { f.store.protectedEnrollment(captured, partition()) } }
    }
}
