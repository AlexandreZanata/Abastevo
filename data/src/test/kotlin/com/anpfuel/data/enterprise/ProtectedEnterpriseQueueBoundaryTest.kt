package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.data.enterprise.ProtectedQueueFixture.ack
import com.anpfuel.data.enterprise.ProtectedQueueFixture.attempt
import com.anpfuel.data.enterprise.ProtectedQueueFixture.id
import com.anpfuel.data.enterprise.ProtectedQueueFixture.operation
import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.C
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.sync.QueueRetryState
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import java.io.IOException
import java.nio.ByteBuffer
import java.nio.file.Path
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.*
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

class ProtectedEnterpriseQueueBoundaryTest {
    @TempDir lateinit var root: Path
    private fun generations(f: VaultFixture) = EnterpriseVaultGenerations(f.files, f.keys)
    private fun head(f: VaultFixture) = generations(f).head(EnterpriseVaultCodec.token(partition()))

    @Test fun entryPressureRefusesNewWorkWithoutEvictingAnyKindAndRetainsFinalReceipts(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val snapshot = generations(f).load(partition())!!
        repeat(7998) { index -> val id = id(index + 1000); snapshot.entries["RECORD:$id"] = VaultEntry(VaultKind.RECORD, id, "", byteArrayOf(7)) }
        generations(f).commit(snapshot)
        val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f); val operation = operation()
        assertFalse(queue.stats(captured).pressure)
        queue.save(captured, operation) { it.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(8))) }
        assertTrue(queue.stats(captured).pressure)
        val full = generations(f).load(partition())!!
        var nextId = 20000
        while (full.entries.size < EnterpriseVaultCodec.MAX_ENTRIES) {
            val key = id(nextId++); full.entries["RECORD:$key"] = VaultEntry(VaultKind.RECORD, key, "", byteArrayOf(7))
        }
        generations(f).commit(full)
        val committed = head(f)
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking {
            queue.save(captured, operation(operationId = id(101))) { it.put(EnterpriseLocalRecord(partition(), B, byteArrayOf(9))) }
        } }
        assertEquals(committed, head(f)); assertEquals(10000, queue.stats(captured).totalEntries)
        assertEquals(1, queue.pending(captured).size); assertNull(f.store.record(partition(), B))
        assertTrue(queue.acknowledge(captured, operation, ack(operation)) { it.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(10))) })
        assertEquals(10000, queue.stats(captured).totalEntries)
        assertTrue(queue.pending(captured).isEmpty()); assertNotNull(queue.entry(captured, operation.operationId)!!.acknowledgement)
    }

    @Test fun bytePressureUsesTheAcceptedAllocationBudgetAndNeverDropsAnAttachment(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val size = EnterpriseVaultCodec.MAX_PLAIN * 4 / 5
        f.store.putPrivateFile(partition(), C, ByteArray(size) { 7 })
        val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f); val committed = head(f)
        val before = queue.stats(captured)
        assertTrue(before.pressure); assertEquals(size.toLong() + 512 + 2048, before.totalBytes)
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking {
            queue.save(captured, operation()) { it.put(EnterpriseLocalRecord(partition(), A, ByteArray(EnterpriseVaultCodec.MAX_PLAIN))) }
        } }
        assertEquals(committed, head(f)); assertEquals(before.totalBytes, queue.stats(captured).totalBytes)
        assertTrue(queue.pending(captured).isEmpty()); assertNull(f.store.record(partition(), A))
        assertEquals(size, f.store.privateFile(partition(), C)!!.size)
    }

    @Test fun fourLegacyOrdinalsRemainReadableButAnUnboundSnapshotCannotEmitTypedWork(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val lease = generations(f).load(partition())!!.lease
        val legacy = VaultSnapshot(partition(), lease)
        for ((index, kind) in listOf(VaultKind.RECORD, VaultKind.OPERATION, VaultKind.ATTACHMENT, VaultKind.CACHE).withIndex()) {
            val id = id(1000 + index)
            legacy.entries["${kind.name}:$id"] = VaultEntry(kind, id, if (kind == VaultKind.OPERATION) A else "", byteArrayOf(index.toByte()))
        }
        val key = f.payloadKey().secret
        val encoded = EnterpriseVaultCodec.seal(legacy, key)
        val read = EnterpriseVaultCodec.open(encoded, partition(), key)
        assertNull(read.bound)
        assertEquals(listOf(0, 1, 2, 3), read.entries.values.map { it.kind.ordinal })
        assertEquals(4, VaultKind.QUEUE.ordinal)
        legacy.entries["QUEUE:${id(101)}"] = VaultEntry(VaultKind.QUEUE, id(101), "", byteArrayOf(1))
        assertThrows(IllegalArgumentException::class.java) { EnterpriseVaultCodec.seal(legacy, key) }
        assertEquals(4, EnterpriseVaultCodec.open(encoded, partition(), key).entries.size)
    }

    @Test fun unknownFrameAndEntryIdentityMismatchStayPreservedWithoutRepairOrErasure(): Unit = runBlocking {
        for (fault in listOf("future", "wrong_id", "trailing")) {
            val f = VaultFixture(root.resolve(fault)); f.confirm(partition())
            val queue = ProtectedEnterpriseQueue(f.store); val operation = operation(); val captured = attempt(f)
            queue.save(captured, operation) { it.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(7))) }
            val snapshot = generations(f).load(partition())!!
            val old = snapshot.entries.getValue("QUEUE:" + operation.operationId)
            val bytes = when (fault) {
                "future" -> old.bytes.copyOf().also { ByteBuffer.wrap(it).putInt(4, 3) }
                "trailing" -> old.bytes + byteArrayOf(0)
                else -> old.bytes.copyOf()
            }
            snapshot.entries.remove("QUEUE:" + operation.operationId)
            val entryId = if (fault == "wrong_id") id(101) else operation.operationId
            snapshot.entries["QUEUE:$entryId"] = VaultEntry(VaultKind.QUEUE, entryId, "", bytes)
            generations(f).commit(snapshot)
            val committed = head(f); val aliases = f.keys.values.keys.toSet()
            assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { queue.pending(captured) } }
            assertEquals(committed, head(f)); assertEquals(aliases, f.keys.values.keys)
            assertFalse(f.erasure.cleanupRequired); assertFalse(f.files.denied(EnterpriseVaultCodec.token(partition())))
            assertArrayEquals(byteArrayOf(7), f.store.record(partition(), A)!!.envelope)
            f.server++; f.confirm(partition())
            assertArrayEquals(bytes, generations(f).load(partition())!!.entries.getValue("QUEUE:$entryId").bytes)
        }
    }

    @Test fun collidingOperationIdsAcrossTwoOrganizationsAccountsAndOriginsNeverCrossProject(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); val scopes = listOf(partition(), partition(org = B), partition(owner = B), partition(origin = "https://other.example.invalid"))
        val queue = ProtectedEnterpriseQueue(f.store)
        for ((index, p) in scopes.withIndex()) {
            f.identity = EnterpriseLocalIdentity(p.ownerAccount, p.origin, A); f.confirm(p)
            queue.save(attempt(f, p), operation(p, value = index.toString())) { it.put(EnterpriseLocalRecord(p, A, byteArrayOf(index.toByte()))) }
        }
        for ((index, p) in scopes.withIndex()) {
            f.identity = EnterpriseLocalIdentity(p.ownerAccount, p.origin, A)
            val captured = attempt(f, p); val stored = queue.pending(captured).single().operation
            assertArrayEquals(operation(p, value = index.toString()).canonicalBytes, stored.canonicalBytes)
            assertArrayEquals(byteArrayOf(index.toByte()), f.store.record(p, A)!!.envelope)
            if (index == 0) queue.acknowledge(captured, stored, ack(stored)) { it.put(EnterpriseLocalRecord(p, A, byteArrayOf(9))) }
            assertEquals(if (index == 0) 0 else 1, queue.pending(captured).size)
        }
        f.identity = EnterpriseLocalIdentity(A, partition().origin, A)
        val foreign = attempt(f, partition(org = B))
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { queue.save(foreign, operation()) { fail("Cross-org projection") } } }
        assertArrayEquals(byteArrayOf(9), f.store.record(partition(), A)!!.envelope)
    }

    @Test fun twelveConcurrentSavesAndFinalizersApplyEachActualProjectionOnce(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f); val operation = operation()
        val saves = AtomicInteger(); val receipts = AtomicInteger()
        val outcomes = coroutineScope { (1..12).map { async(Dispatchers.Default) {
            queue.save(captured, operation) { tx -> saves.incrementAndGet(); tx.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(1))) }
        } }.awaitAll() }
        assertEquals(1, outcomes.count { it }); assertEquals(1, saves.get())
        val final = ack(operation)
        val finalized = coroutineScope { (1..12).map { async(Dispatchers.Default) {
            queue.acknowledge(captured, operation, final) { tx -> receipts.incrementAndGet(); tx.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(2))) }
        } }.awaitAll() }
        assertEquals(1, finalized.count { it }); assertEquals(1, receipts.get())
        assertArrayEquals(final.canonicalBytes, queue.entry(captured, operation.operationId)!!.acknowledgement!!.canonicalBytes)
        assertArrayEquals(byteArrayOf(2), f.store.record(partition(), A)!!.envelope)
        coroutineScope { (200..211).map { value -> async(Dispatchers.Default) {
            queue.save(captured, operation(operationId = id(value))) { tx ->
                val current = tx.record(B)?.envelope?.let { ByteBuffer.wrap(it).int } ?: 0
                tx.put(EnterpriseLocalRecord(partition(), B, ByteBuffer.allocate(4).putInt(current + 1).array()))
            }
        } }.awaitAll() }
        assertEquals(12, ByteBuffer.wrap(f.store.record(partition(), B)!!.envelope).int)
        assertEquals(12, queue.pending(captured).size)
    }

    @Test fun aCancelledProjectorCannotCommitItsTentativeRecordOrOperation(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f); val committed = head(f)
        assertThrows(CancellationException::class.java) { runBlocking {
            queue.save(captured, operation()) { it.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(9))); throw CancellationException("synthetic cancellation") }
        } }
        assertEquals(committed, head(f)); assertTrue(queue.pending(captured).isEmpty()); assertNull(f.store.record(partition(), A))
    }

    @Test fun clockChangingDuringFinalIoCannotReturnAnAuthorizedSave(): Unit = runBlocking {
        for (scenario in listOf("expiry", "reboot")) {
            var armed = false
            var advance: () -> Unit = { }
            val f = VaultFixture(root.resolve(scenario)) { real -> object : EnterpriseVaultFiles by real {
                override fun replaceGeneration(token: String, generation: Long, descriptor: Boolean, ciphertext: ByteArray) {
                    real.replaceGeneration(token, generation, descriptor, ciphertext)
                    if (armed && !descriptor) { armed = false; advance() }
                }
            } }
            f.confirm(partition())
            val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f); val operation = operation()
            advance = {
                f.now = if (scenario == "expiry") f.now.copy(elapsedMillis = f.now.elapsedMillis + EnterpriseOfflineLease.DEFAULT_VALIDITY_MILLIS)
                    else f.now.copy(boot = "synthetic-next-boot", elapsedMillis = 0)
            }
            armed = true
            assertThrows(EnterpriseLocalDenied::class.java) { runBlocking {
                queue.save(captured, operation) { it.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(7))) }
            } }
            if (scenario == "expiry") {
                assertTrue(f.contentKeys().isEmpty())
                assertTrue(f.files.generations(EnterpriseVaultCodec.token(partition())).isEmpty())
                assertFalse(f.erasure.cleanupRequired)
            } else {
                assertNotNull(head(f), "The already-committed original work is retained while reconfirmation is required")
                assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { queue.pending(attempt(f)) } }
                f.server++; f.confirm(partition())
                assertArrayEquals(operation.canonicalBytes, queue.pending(attempt(f)).single().operation.canonicalBytes)
                assertArrayEquals(byteArrayOf(7), f.store.record(partition(), A)!!.envelope)
            }
        }
    }

    @Test fun olderConcurrentRetryCannotShortenANewerRetryAfterFloor(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f); val operation = operation()
        queue.save(captured, operation) { }
        val oldRead = CompletableDeferred<Unit>(); val newerStored = CompletableDeferred<Unit>()
        withTimeout(5000) {
            coroutineScope {
                val older = async(Dispatchers.Default) {
                    val expected = queue.entry(captured, operation.operationId)!!.retry
                    assertEquals(0L, expected.attempts)
                    oldRead.complete(Unit)
                    newerStored.await()
                    queue.retry(captured, operation, expected, QueueRetryState(1, f.now.copy(elapsedMillis = f.now.elapsedMillis + 1), 30_000))
                }
                oldRead.await()
                val expected = queue.entry(captured, operation.operationId)!!.retry
                assertEquals(0L, expected.attempts)
                assertTrue(queue.retry(captured, operation, expected, QueueRetryState(1, f.now, 3_600_000)))
                newerStored.complete(Unit)
                assertFalse(older.await())
            }
        }
        assertEquals(3_600_000L, queue.entry(captured, operation.operationId)!!.retry.delayMillis,
            "A late response from the older captured state cannot overwrite the actual newer retry floor")
    }

    @Test fun everyReceiptProjectionAndPreparationFailureRetainsTheOriginalPendingGeneration(): Unit = runBlocking {
        for (stage in listOf("project", "data", "descriptor", "head")) {
            var armed = false
            val keys = TestVaultKeys()
            val f = VaultFixture(root.resolve(stage), keys) { real -> object : EnterpriseVaultFiles by real {
                override fun replaceGeneration(token: String, generation: Long, descriptor: Boolean, ciphertext: ByteArray) {
                    if (armed && stage == if (descriptor) "descriptor" else "data") throw IOException("synthetic file failure")
                    real.replaceGeneration(token, generation, descriptor, ciphertext)
                }
            } }
            f.confirm(partition())
            val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f); val operation = operation()
            queue.save(captured, operation) { it.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(7))) }
            val committed = head(f)
            keys.afterCreate = { alias -> if (armed && stage == "head" && alias.endsWith("_m")) keys.failCreate = true }
            armed = true
            assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking {
                queue.acknowledge(captured, operation, ack(operation)) {
                    it.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(8)))
                    if (stage == "project") error("synthetic owning projector failure")
                }
            } }
            armed = false; keys.failCreate = false; keys.afterCreate = null
            assertEquals(committed, head(f)); assertNull(queue.entry(captured, operation.operationId)!!.acknowledgement)
            assertArrayEquals(byteArrayOf(7), f.store.record(partition(), A)!!.envelope)
            assertEquals(1, queue.pending(captured).size)
            assertTrue(queue.acknowledge(captured, operation, ack(operation)) { it.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(8))) })
        }
    }

    @Test fun backgroundAfterTrustedHeadWithholdsSuccessButNeverClaimsCommittedWorkWasRolledBack(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        f.confirm(partition(org = B)); f.store.put(EnterpriseLocalRecord(partition(org = B), A, byteArrayOf(6)))
        val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f); val operation = operation()
        var armed = true
        f.keys.afterCreate = { alias -> if (armed && alias.endsWith("_h")) { armed = false; f.fence.markBackground() } }
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking {
            queue.save(captured, operation) { it.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(7))) }
        } }
        f.keys.afterCreate = null
        val fresh = attempt(f)
        assertArrayEquals(operation.canonicalBytes, queue.pending(fresh).single().operation.canonicalBytes)
        assertArrayEquals(byteArrayOf(7), f.store.record(partition(), A)!!.envelope)
        assertFalse(queue.save(fresh, operation) { fail("Durable old-scope projection was reapplied") })
        assertArrayEquals(byteArrayOf(6), f.store.record(partition(org = B), A)!!.envelope)
    }

    @Test fun managedUnlockAndGenericContactOutagePreserveQueueUntilActualCustodyExpiry(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.unlock = 60; f.confirm(partition())
        val queue = ProtectedEnterpriseQueue(f.store); var captured = attempt(f); val operation = operation()
        queue.save(captured, operation) { it.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(7))) }
        val committed = head(f)
        f.keys.locked = true
        assertThrows(EnterpriseUnlockRequired::class.java) { runBlocking { queue.pending(captured) } }
        assertEquals(committed, head(f)); assertFalse(f.erasure.cleanupRequired)
        f.keys.locked = false; f.denyContact = true
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { f.confirm(partition()) } }
        captured = attempt(f)
        assertArrayEquals(operation.canonicalBytes, queue.pending(captured).single().operation.canonicalBytes)
        assertArrayEquals(byteArrayOf(7), f.store.record(partition(), A)!!.envelope)
        f.now = f.now.copy(elapsedMillis = f.now.elapsedMillis + EnterpriseOfflineLease.DEFAULT_VALIDITY_MILLIS)
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { queue.pending(captured) } }
        assertTrue(f.contentKeys().isEmpty()); assertTrue(f.files.generations(EnterpriseVaultCodec.token(partition())).isEmpty())
        assertFalse(f.erasure.cleanupRequired)
    }

    @Test fun discoveryReportsOneUnavailableDescriptorWithoutOpeningOrBlockingAnotherOrganization(): Unit = runBlocking {
        var broken = false
        val failedToken = EnterpriseVaultCodec.token(partition())
        val f = VaultFixture(root.resolve("vault")) { real -> object : EnterpriseVaultFiles by real {
            override fun readGeneration(token: String, generation: Long, descriptor: Boolean): ByteArray? {
                if (broken && token == failedToken && descriptor) throw IOException("synthetic descriptor outage")
                return real.readGeneration(token, generation, descriptor)
            }
        } }
        f.confirm(partition()); f.confirm(partition(org = B))
        val source = ProtectedQueueFixture.account(f); val aliases = f.keys.values.keys.toSet()
        broken = true
        val page = f.store.discover(source)
        assertEquals(1, page.unavailable); assertEquals(listOf(B), page.enrollments.map { it.target.partition.organizationId })
        assertNull(page.nextAfterOpaque); assertEquals(aliases, f.keys.values.keys)
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { f.store.protectedEnrollment(source, partition()) } }
        assertEquals(B, f.store.protectedEnrollment(source, partition(org = B)).target.partition.organizationId)
        broken = false
        assertEquals(2, f.store.discover(source).enrollments.size)
    }

    @Test fun reviewAndRefusalReceiptsAreRetainedFinalFactsWithoutRetryDowngrade(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); f.confirm(partition())
        val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f)
        for ((index, result) in listOf("ACCEPTED_FOR_REVIEW", "REJECTED").withIndex()) {
            val operation = operation(operationId = id(100 + index))
            queue.save(captured, operation) { it.put(EnterpriseLocalRecord(partition(), operation.operationId, byteArrayOf(7))) }
            val code = if (index == 0) "ORPHAN_REFERENCE" else "MALFORMED"
            val review = if (index == 0) "\"${id(800)}\"" else "null"
            val ack = OperationAcknowledgement.parse("""{"schema_version":1,"operation_id":"${operation.operationId}","payload_digest":"${operation.payloadDigest}","result":"$result","code":"$code","entity_revision":null,"received_at":"2026-11-13T12:00:01Z","review_item_id":$review,"warnings":[]}""".toByteArray(), operation)
            queue.acknowledge(captured, operation, ack) { it.put(EnterpriseLocalRecord(partition(), operation.operationId, ack.canonicalBytes)) }
            val committed = head(f)
            assertFalse(queue.retry(captured, operation, QueueRetryState(), QueueRetryState(1, f.now, 30_000)))
            assertEquals(committed, head(f)); assertArrayEquals(ack.canonicalBytes, queue.entry(captured, operation.operationId)!!.acknowledgement!!.canonicalBytes)
            assertArrayEquals(ack.canonicalBytes, f.store.record(partition(), operation.operationId)!!.envelope)
        }
        assertTrue(queue.pending(captured).isEmpty()); assertEquals(4, queue.stats(captured).totalEntries)
    }
}
