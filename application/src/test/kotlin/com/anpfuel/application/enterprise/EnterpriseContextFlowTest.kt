package com.anpfuel.application.enterprise

import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterprisePendingOperation
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Deferred
import kotlinx.coroutines.async
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

@OptIn(kotlinx.coroutines.ExperimentalCoroutinesApi::class)
class EnterpriseContextFlowTest {
    private val a = "7b0e2a54-0000-4000-8000-000000000001"
    private val b = "7b0e2a54-0000-4000-8000-000000000002"
    private val c = "7b0e2a54-0000-4000-8000-000000000003"
    private val binding = EnterpriseAccessBinding(a, "https://api.example.invalid", a, 0)
    private fun summary(org: String = a, owner: String = a, origin: String = binding.origin) = EnterpriseContextSummary(
        EnterprisePartition.of(owner, origin, org), b, "Synthetic company $org",
    )

    private open inner class Gateway : EnterpriseContextGateway {
        var selected: EnterpriseAccessBinding? = binding
        var page: EnterpriseAccessResult<EnterpriseContextPage> = EnterpriseAccessResult.Ok(EnterpriseContextPage(listOf(summary(), summary(b))))
        var readResult: EnterpriseAccessResult<EnterpriseContextSummary>? = null
        val reads = mutableListOf<EnterpriseContextSummary>()
        val pages = mutableListOf<String?>()
        var opens = 0
        override fun currentBinding(): EnterpriseAccessBinding? = selected
        override suspend fun open(): EnterpriseAccessResult<EnterpriseAccessBinding> {
            opens++; return selected?.let { EnterpriseAccessResult.Ok(it) } ?: EnterpriseAccessResult.Err(EnterpriseAccessError.SIGN_IN_REQUIRED)
        }
        override suspend fun list(binding: EnterpriseAccessBinding, afterOrganizationId: String?): EnterpriseAccessResult<EnterpriseContextPage> {
            pages += afterOrganizationId; return page
        }
        override suspend fun read(binding: EnterpriseAccessBinding, context: EnterpriseContextSummary): EnterpriseAccessResult<EnterpriseContextSummary> {
            reads += context; return readResult ?: EnterpriseAccessResult.Ok(context)
        }
        override fun invalidate() = Unit
    }

    private open class Memory : EnterprisePersistence {
        val records = mutableMapOf<Pair<EnterprisePartition, String>, EnterpriseLocalRecord>()
        val operations = linkedMapOf<Pair<EnterprisePartition, String>, EnterprisePendingOperation>()
        var calls = 0
        override suspend fun record(partition: EnterprisePartition, id: String): EnterpriseLocalRecord? { calls++; return records[partition to id] }
        override suspend fun put(record: EnterpriseLocalRecord) { calls++; records[record.partition to record.id] = record }
        override suspend fun enqueue(operation: EnterprisePendingOperation): Boolean { calls++; return operations.putIfAbsent(operation.partition to operation.id, operation) == null }
        override suspend fun pending(partition: EnterprisePartition, limit: Int): List<EnterprisePendingOperation> { calls++; return operations.values.filter { it.partition == partition }.take(limit) }
    }

    private fun local(gateway: Gateway, memory: EnterprisePersistence): EnterpriseLocalContext = EnterpriseLocalContext(
        { gateway.selected?.accountId }, { gateway.selected?.origin ?: binding.origin }, memory, { gateway.selected?.epoch ?: -1 },
    )

    @Test fun `selection requires current read and switching never reads retags or moves queued operations`(): Unit = runTest {
        val gateway = Gateway()
        val memory = Memory()
        val local = local(gateway, memory)
        val old = local.select(summary().partition)
        old.enqueue(EnterprisePendingOperation(old.partition, a, b, byteArrayOf(1)))
        val flow = EnterpriseContextFlow(gateway, local)
        assertNull(flow.state.value.selected)
        flow.refresh()
        assertTrue(flow.state.value.catalogConfirmed)
        assertNull(flow.state.value.selected)
        flow.select(summary(b))
        assertEquals(listOf(summary(b)), gateway.reads)
        assertEquals(summary(b), flow.state.value.selected)
        assertTrue(flow.state.value.selectionConfirmed)
        assertEquals(1, memory.calls)
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { old.pending() } }
        assertTrue(local.select(summary(b).partition).pending().isEmpty())
        flow.select(summary())
        val original = local.select(summary().partition).pending().single()
        assertEquals(summary().partition, original.partition)
        assertEquals(a, original.id)
        assertArrayEquals(byteArrayOf(1), original.envelope)
        flow.selectPersonal()
        assertNull(flow.state.value.selected)
        assertEquals(0L, gateway.selected!!.epoch)
    }

    @Test fun `offline and background retain only same identity metadata unconfirmed with no surviving handle`(): Unit = runTest {
        val gateway = Gateway()
        val memory = Memory()
        val local = local(gateway, memory)
        val flow = EnterpriseContextFlow(gateway, local)
        flow.refresh(); flow.select(summary())
        val handle = local.select(summary().partition)
        flow.unconfirm()
        assertEquals(summary(), flow.state.value.selected)
        assertFalse(flow.state.value.selectionConfirmed)
        assertFalse(flow.state.value.catalogConfirmed)
        assertNull(flow.state.value.nextAfterOrganizationId)
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { handle.pending() } }
        assertEquals(0L, gateway.selected!!.epoch)
        flow.refresh()
        assertTrue(flow.state.value.selectionConfirmed)
        gateway.selected = binding.copy(epoch = 1)
        flow.invalidateIfStale()
        assertEquals(summary(), flow.state.value.selected)
        assertFalse(flow.state.value.selectionConfirmed)
        assertFalse(flow.state.value.catalogConfirmed)
    }

    @Test fun `account family origin and logout clear all private context metadata`(): Unit = runTest {
        for (changed in listOf(binding.copy(accountId = b), binding.copy(familyId = b), binding.copy(origin = "https://other.example.invalid"), null)) {
            val gateway = Gateway()
            val flow = EnterpriseContextFlow(gateway, local(gateway, Memory()))
            flow.refresh(); flow.select(summary())
            gateway.selected = changed
            flow.invalidateIfStale()
            assertTrue(flow.state.value.contexts.isEmpty())
            assertNull(flow.state.value.selected)
            assertFalse(flow.state.value.selectionConfirmed)
            assertEquals(changed != null, flow.state.value.signedIn)
        }
    }

    @Test fun `definitive denial clears cache while transport failure retains unconfirmed metadata`(): Unit = runTest {
        for (error in listOf(EnterpriseAccessError.ACCESS_DENIED, EnterpriseAccessError.UNAVAILABLE)) {
            val gateway = Gateway()
            val flow = EnterpriseContextFlow(gateway, local(gateway, Memory()))
            flow.refresh(); flow.select(summary())
            gateway.page = EnterpriseAccessResult.Err(error)
            flow.refresh()
            assertEquals(error, flow.state.value.error)
            assertFalse(flow.state.value.selectionConfirmed)
            if (error == EnterpriseAccessError.ACCESS_DENIED) {
                assertNull(flow.state.value.selected)
                assertTrue(flow.state.value.contexts.isEmpty())
            } else {
                assertEquals(summary(), flow.state.value.selected)
                assertEquals(2, flow.state.value.contexts.size)
            }
        }
    }

    @Test fun `read denial foreign selectors and foreign acknowledgements never select a partition`(): Unit = runTest {
        val gateway = Gateway()
        val flow = EnterpriseContextFlow(gateway, local(gateway, Memory()))
        flow.refresh(); flow.select(summary())
        flow.select(summary(b, owner = b))
        assertEquals(EnterpriseAccessError.ACCESS_DENIED, flow.state.value.error)
        assertNull(flow.state.value.selected)
        gateway.readResult = EnterpriseAccessResult.Ok(summary(b))
        flow.select(summary())
        assertEquals(EnterpriseAccessError.ACCESS_DENIED, flow.state.value.error)
        assertNull(flow.state.value.selected)
        gateway.readResult = EnterpriseAccessResult.Err(EnterpriseAccessError.ACCESS_DENIED)
        flow.select(summary())
        assertTrue(flow.state.value.contexts.isEmpty())
        assertNull(flow.state.value.selected)
    }

    @Test fun `late selected organization response cannot reopen after personal or a newer selection`(): Unit = runTest {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val gateway = object : Gateway() {
            override suspend fun read(binding: EnterpriseAccessBinding, context: EnterpriseContextSummary): EnterpriseAccessResult<EnterpriseContextSummary> {
                if (context.partition.organizationId == a) { entered.complete(Unit); release.await() }
                return super.read(binding, context)
            }
        }
        val flow = EnterpriseContextFlow(gateway, local(gateway, Memory()))
        flow.refresh()
        val old = async { flow.select(summary()) }
        entered.await()
        flow.unconfirm()
        flow.select(summary(b))
        release.complete(Unit)
        old.await()
        assertEquals(summary(b), flow.state.value.selected)
        assertTrue(flow.state.value.selectionConfirmed)
        flow.selectPersonal()
        assertNull(flow.state.value.selected)
    }

    @Test fun `bounded pages continue empty scans and reject backward cursors without accumulating`(): Unit = runTest {
        val gateway = Gateway().apply { page = EnterpriseAccessResult.Ok(EnterpriseContextPage(emptyList(), a)) }
        val flow = EnterpriseContextFlow(gateway, local(gateway, Memory()))
        flow.refresh()
        assertTrue(flow.state.value.contexts.isEmpty())
        assertEquals(a, flow.state.value.nextAfterOrganizationId)
        gateway.page = EnterpriseAccessResult.Ok(EnterpriseContextPage(listOf(summary(b)), b))
        flow.loadMore()
        assertEquals(listOf(summary(b)), flow.state.value.contexts)
        assertEquals(listOf(null, a), gateway.pages)
        gateway.page = EnterpriseAccessResult.Ok(EnterpriseContextPage(listOf(summary(c))))
        flow.loadMore()
        assertEquals(listOf(summary(c)), flow.state.value.contexts)
        assertNull(flow.state.value.nextAfterOrganizationId)
        flow.refresh()
        gateway.page = EnterpriseAccessResult.Ok(EnterpriseContextPage(emptyList(), a))
        flow.refresh()
        flow.loadMore()
        assertEquals(EnterpriseAccessError.UNAVAILABLE, flow.state.value.error)
        assertFalse(flow.state.value.catalogConfirmed)
    }

    @Test fun `cancellation close and fresh process keep held queues but clear presentation`(): Unit = runTest {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val gateway = object : Gateway() {
            override suspend fun read(binding: EnterpriseAccessBinding, context: EnterpriseContextSummary): EnterpriseAccessResult<EnterpriseContextSummary> {
                entered.complete(Unit); release.await(); return super.read(binding, context)
            }
        }
        val memory = Memory()
        val local = local(gateway, memory)
        local.select(summary().partition).enqueue(EnterprisePendingOperation(summary().partition, a, b, byteArrayOf(3)))
        val flow = EnterpriseContextFlow(gateway, local)
        flow.refresh()
        val selecting = launch { flow.select(summary()) }
        entered.await(); selecting.cancelAndJoin()
        assertFalse(flow.state.value.busy)
        assertFalse(flow.state.value.selectionConfirmed)
        flow.close()
        assertTrue(flow.state.value.contexts.isEmpty())
        assertNull(flow.state.value.selected)
        val opens = gateway.opens
        flow.refresh()
        assertEquals(opens, gateway.opens)
        val recreated = EnterpriseContextFlow(gateway, local)
        assertNull(recreated.state.value.selected)
        assertTrue(recreated.state.value.contexts.isEmpty())
        assertEquals(1, memory.operations.size)
        assertFalse(recreated.state.value.toString().contains(a))
    }

    @Test fun `metadata shell never opens protected storage or claims a persisted default selection`(): Unit = runTest {
        val gateway = Gateway()
        val flow = EnterpriseContextFlow(gateway, local(gateway, UnavailableEnterprisePersistence))
        flow.refresh(); flow.select(summary())
        assertTrue(flow.state.value.selectionConfirmed)
        assertEquals(summary().partition, flow.state.value.selected!!.partition)
        assertNull(EnterpriseContextFlow(gateway, local(gateway, UnavailableEnterprisePersistence)).state.value.selected)
    }

    @Test fun `outgoing flow close never invalidates incoming flow selection or its feature handle`(): Unit = runTest {
        val gatewayA = Gateway()
        val gatewayB = Gateway()
        val local = local(gatewayA, Memory())
        val old = EnterpriseContextFlow(gatewayA, local)
        val newest = EnterpriseContextFlow(gatewayB, local)
        old.select(summary())
        old.unconfirm()
        newest.select(summary(b))
        val featureB = local.select(summary(b).partition)
        old.close()
        assertTrue(newest.state.value.selectionConfirmed)
        assertTrue(featureB.isCurrent())
        assertTrue(featureB.pending().isEmpty())
        newest.close()
        assertFalse(featureB.isCurrent())
    }

    @Test fun `late personal switch waiting for local IO is harmless after a newer flow selects`(): Unit = runTest {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val memory = object : Memory() {
            override suspend fun record(partition: EnterprisePartition, id: String): EnterpriseLocalRecord {
                entered.complete(Unit); release.await()
                return EnterpriseLocalRecord(partition, id, byteArrayOf(1))
            }
        }
        val gatewayA = Gateway()
        val gatewayB = Gateway()
        val local = local(gatewayA, memory)
        val old = EnterpriseContextFlow(gatewayA, local)
        val newest = EnterpriseContextFlow(gatewayB, local)
        old.select(summary())
        val featureA = local.select(summary().partition)
        val reading = async { runCatching { featureA.record(a) } }
        entered.await()
        val personal = async { old.selectPersonal() }
        runCurrent()
        assertNull(old.state.value.selected)
        assertFalse(personal.isCompleted)
        val selectingB = async { newest.select(summary(b)) }
        runCurrent()
        release.complete(Unit)
        assertTrue(reading.await().exceptionOrNull() is EnterpriseLocalDenied)
        personal.await()
        selectingB.await()
        val featureB = local.select(summary(b).partition)
        old.close()
        assertTrue(newest.state.value.selectionConfirmed)
        assertTrue(featureB.isCurrent())
        assertTrue(featureB.pending().isEmpty())
    }

    @Test fun `same flow late post-selection check cannot revoke its newer reserved selection lease`(): Unit = runTest {
        val gateway = Gateway()
        val memory = Memory()
        lateinit var flow: EnterpriseContextFlow
        var reserveNext = true
        var newer: Deferred<Unit>? = null
        val local = EnterpriseLocalContext(
            ownerAccount = {
                if (reserveNext) {
                    reserveNext = false
                    // The real old select holds its mutex after consuming its lease. Reserve a newer
                    // attempt before it returns; that attempt suspends on the same local mutex.
                    flow.unconfirm()
                    newer = async(start = CoroutineStart.UNDISPATCHED) { flow.select(summary(b)) }
                }
                a
            },
            serverOrigin = { binding.origin }, persistence = memory,
        )
        flow = EnterpriseContextFlow(gateway, local)
        flow.select(summary())
        newer!!.await()
        assertEquals(summary(b), flow.state.value.selected)
        assertTrue(flow.state.value.selectionConfirmed)
        assertNull(flow.state.value.error)
        val featureB = local.select(summary(b).partition)
        assertTrue(featureB.isCurrent())
        assertTrue(featureB.pending().isEmpty())
    }
}
