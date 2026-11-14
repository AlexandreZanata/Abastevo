package com.anpfuel.domain.enterprise

import com.anpfuel.domain.enterprise.sync.*
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseQueueTest {
    private fun id(n: Int) = "7c000000-0000-4000-8000-" + n.toString().padStart(12, '0')
    private fun op(n: Int, dependencies: List<Int> = emptyList(), organization: Int = 3): EnterpriseOperation {
        val raw = """{"schema_version":1,"operation_id":"${id(n)}","owner_account_id":"${id(1)}","api_origin":"https://sync.example.invalid","org_id":"${id(organization)}","membership_id":"${id(4)}","enrollment_id":"${id(5)}","type":"validation.queue","operation_schema":1,"entity":{"type":"record","id":"${id(n + 100)}","expected_revision":null},"occurred_at":"2026-11-13T00:00:00Z","entry_zone":"UTC","device_offset_seconds":0,"depends_on":[${dependencies.joinToString { "\"${id(it)}\"" }}],"soft_references":[],"attachments":[],"payload":{"value":"synthetic"}}"""
        return EnterpriseOperation.seal(raw.toByteArray(), SyncBinding.of(id(1), "https://sync.example.invalid", id(organization)))
    }
    private fun ack(operation: EnterpriseOperation): ByteArray = """{"schema_version":1,"operation_id":"${operation.operationId}","payload_digest":"${operation.payloadDigest}","result":"ACCEPTED","code":null,"entity_revision":1,"received_at":"2026-11-13T00:00:01Z","review_item_id":null,"warnings":[]}""".toByteArray()
    private fun clock(boot: String = "boot", elapsed: Long = 1000) = EnterpriseDeviceClock(boot, elapsed, 10000)

    @Test fun batchEncodingKeepsExactIntentAndFitsActualEscapedDeliveryProof() {
        val operations = listOf(op(10), op(11, listOf(10)))
        val encoded = SyncBatch.encode(id(8), "\\\"token", operations)
        val decoded = SyncBatch.parse(encoded)
        assertEquals("\\\"token", decoded.accessToken)
        decoded.operations.zip(operations).forEach { (candidate, original) ->
            assertArrayEquals(original.canonicalBytes, candidate.parse(original.binding).canonicalBytes)
        }
        assertThrows(SyncProtocolException::class.java) { SyncBatch.encode(id(8), "token", listOf(operations[0], operations[0])) }
        assertThrows(SyncProtocolException::class.java) { SyncBatch.encode(id(8), "token", listOf(operations[0], op(12, organization = 9))) }
        assertThrows(SyncProtocolException::class.java) { SyncBatch.encode(id(8), "token", operations, SyncLimits.trusted(maxOperations = 1)) }
        assertThrows(SyncProtocolException::class.java) { SyncBatch.encode(id(8), "token", operations, SyncLimits.trusted(maxBatchBytes = 1000, maxOperationBytes = 1000)) }
    }

    @Test fun completeResponseIsPreflightedBeforeAnyAcknowledgementCanEscape() {
        val operations = listOf(op(10), op(11))
        val raw = """{"schema_version":1,"acknowledgements":[${String(ack(operations[0]))},${String(ack(operations[1]))}]}""".toByteArray()
        val response = SyncBatchResponse.parse(raw, operations)
        assertEquals(2, response.acknowledgements.size)
        assertTrue(response.acknowledgements[1].matches(operations[1]))
        assertThrows(SyncProtocolException::class.java) { SyncBatchResponse.parse(raw, operations.reversed()) }
        assertThrows(SyncProtocolException::class.java) { SyncBatchResponse.parse(raw, listOf(operations[0])) }
        assertThrows(SyncProtocolException::class.java) { SyncBatchResponse.parse(raw + "x".toByteArray(), operations) }
        assertThrows(SyncProtocolException::class.java) { SyncBatchResponse.parse("""{"schema_version":1,"acknowledgements":[${String(ack(operations[0]))},${String(ack(operations[0]))}]}""".toByteArray(), operations) }
        assertThrows(SyncProtocolException::class.java) { SyncBatchResponse.parse("""{"schema_version":1,"acknowledgements":[],"unknown":true}""".toByteArray(), emptyList()) }
        assertTrue(SyncBatchResponse.parse("""{"schema_version":1,"acknowledgements":[]}""".toByteArray(), emptyList()).acknowledgements.isEmpty())
    }

    @Test fun localParentsOrderBeforeChildrenWhileExternalDependenciesDoNotStrandWork() {
        val child = op(11, listOf(10)); val parent = op(10); val external = op(12, listOf(99))
        val result = EnterpriseQueueOrdering.order(listOf(child, external, parent))
        assertTrue(result.ordered.indexOf(parent) < result.ordered.indexOf(child))
        assertEquals(setOf(parent.operationId, child.operationId, external.operationId), result.ordered.map { it.operationId }.toSet())
        assertTrue(result.cycleBlocked.isEmpty())
        assertThrows(IllegalArgumentException::class.java) { EnterpriseQueueOrdering.order(listOf(parent, parent)) }
    }

    @Test fun cycleAndItsChildrenRemainHeldWhileUnrelatedWorkProgresses() {
        val a = op(10, listOf(11)); val b = op(11, listOf(10)); val child = op(12, listOf(10)); val unrelated = op(13)
        val result = EnterpriseQueueOrdering.order(listOf(a, child, b, unrelated))
        assertEquals(listOf(unrelated.operationId), result.ordered.map { it.operationId })
        assertEquals(setOf(a.operationId, b.operationId, child.operationId), result.cycleBlocked)
    }

    @Test fun equalJitterBackoffSaturatesWithoutCeilingAndNeverShortensRetryAfter() {
        val policy = QueueRetryPolicy()
        assertEquals(15000L, policy.delayMillis(1, { 0 }))
        assertEquals(30000L, policy.delayMillis(1, { it - 1 }))
        assertEquals(30000L, policy.delayMillis(2, { 0 }))
        assertEquals(21600000L, policy.delayMillis(Long.MAX_VALUE, { it - 1 }))
        assertEquals(99999999L, policy.delayMillis(1, { 0 }, 99999999))
        assertEquals(Long.MAX_VALUE, policy.nextAttempts(Long.MAX_VALUE))
        assertThrows(IllegalArgumentException::class.java) { policy.delayMillis(0, { 0 }) }
        assertThrows(IllegalArgumentException::class.java) { policy.delayMillis(1, { -1 }) }
        assertThrows(IllegalArgumentException::class.java) { policy.delayMillis(1, { it }) }
        assertThrows(IllegalArgumentException::class.java) { policy.delayMillis(1, { 0 }, -1) }
    }

    @Test fun retryScheduleUsesSameBootMonotonicTimeAndPressureNeverEvicts() {
        val retry = QueueRetryState(1, clock(), 1000)
        assertFalse(retry.isDue(clock(elapsed = 1999)))
        assertTrue(retry.isDue(clock(elapsed = 2000)))
        assertFalse(retry.isDue(clock("reboot", 9000)))
        assertFalse(retry.isDue(clock(elapsed = 999)))
        assertFalse(QueueRetryState(1, clock(elapsed = Long.MAX_VALUE - 2), 10).isDue(clock(elapsed = Long.MAX_VALUE)))
        assertTrue(QueueRetryState().isDue(clock()))
        assertTrue(EnterpriseQueueStats(1, 1, 0, 8000, 2048).pressure)
        assertFalse(EnterpriseQueueStats(1, 1, 0, 7999, 2048).pressure)
        assertTrue(EnterpriseQueueStats(1, 1, 0, 1, (32L * 1024 * 1024 * 4 + 4) / 5).pressure)
    }

    @Test fun retainedFinalEntryCannotAdoptAnotherOperationOrNonfinalRetryAndDiagnosticsStayPrivate() {
        val operation = op(10)
        val acknowledgement = OperationAcknowledgement.parse(ack(operation), operation)
        val retry = QueueRetryState(1, clock(), 1000)
        val entry = EnterpriseQueueEntry(operation, retry, acknowledgement)
        assertSame(operation, entry.operation); assertSame(retry, entry.retry); assertSame(acknowledgement, entry.acknowledgement)
        val different = op(11)
        assertThrows(IllegalArgumentException::class.java) { EnterpriseQueueEntry(different, retry, acknowledgement) }
        val nonfinal = OperationAcknowledgement.parse("""{"schema_version":1,"operation_id":"${operation.operationId}","payload_digest":"${operation.payloadDigest}","result":"RETRY","code":"UNAVAILABLE","entity_revision":null,"received_at":null,"review_item_id":null,"warnings":[]}""".toByteArray(), operation)
        assertThrows(IllegalArgumentException::class.java) { EnterpriseQueueEntry(operation, retry, nonfinal) }
        assertThrows(IllegalArgumentException::class.java) { QueueRetryState(-1) }
        assertThrows(IllegalArgumentException::class.java) { QueueRetryState(1, null, 1) }
        assertThrows(IllegalArgumentException::class.java) { QueueRetryPolicy(0) }
        assertThrows(IllegalArgumentException::class.java) { QueueRetryPolicy(100, 99) }
        assertThrows(IllegalArgumentException::class.java) { EnterpriseQueueStats(-1, 0, 0, 0, 0) }
        val stats = EnterpriseQueueStats(1, 55, 2, 3, 4000)
        assertEquals(1, stats.queuedCount); assertEquals(55L, stats.queuedBytes); assertEquals(2, stats.legacyCount)
        assertEquals(3, stats.totalEntries); assertEquals(4000L, stats.totalBytes)
        val policy = QueueRetryPolicy()
        assertEquals(30000L, policy.baseMillis); assertEquals(21600000L, policy.capMillis); assertEquals(1L, policy.nextAttempts(0))
        for (value in listOf(entry, retry, stats, SyncBatchResponse.parse("""{"schema_version":1,"acknowledgements":[]}""".toByteArray(), emptyList()))) {
            assertTrue(value.toString().contains("redacted")); assertFalse(value.toString().contains(operation.operationId))
        }
    }

    @Test fun deliveryBudgetRetainsOnlyExactEncodedSizeAcrossGraphicAsciiAndLimits() {
        val operation = op(10)
        for (token in listOf("x", "short", "\\\"", (33..126).map(Int::toChar).joinToString(""), "x".repeat(512), "\\".repeat(512))) {
            val budget = SyncDeliveryBudget.fromProof(id(8), token)
            val exact = SyncBatch.encode(id(8), token, listOf(operation)).size
            assertEquals(exact, budget.batchBytes(listOf(operation)))
            val limits = SyncLimits.trusted(maxBatchBytes = exact, maxOperationBytes = operation.canonicalBytes.size)
            assertEquals(exact, budget.batchBytes(listOf(operation), limits))
            assertThrows(SyncProtocolException::class.java) { budget.batchBytes(listOf(operation),
                SyncLimits.trusted(maxBatchBytes = exact - 1, maxOperationBytes = operation.canonicalBytes.size)) }
            assertEquals("SyncDeliveryBudget(redacted)", budget.toString())
        }
        assertEquals(SyncBatch.encode(id(8), "\\".repeat(512), listOf(operation)).size,
            SyncDeliveryBudget.maximum().batchBytes(listOf(operation)))
        for (token in listOf("", " ", "é", "x".repeat(513)))
            assertThrows(SyncProtocolException::class.java) { SyncDeliveryBudget.fromProof(id(8), token) }
        assertThrows(SyncProtocolException::class.java) { SyncDeliveryBudget.fromProof("not-a-family", "x") }
    }
}
