package com.anpfuel.application.enterprise

import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterprisePendingOperation
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

@OptIn(kotlinx.coroutines.ExperimentalCoroutinesApi::class)
class EnterpriseLocalContextTest {
    private val a = "7b0e2a54-0000-4000-8000-000000000001"
    private val b = "7b0e2a54-0000-4000-8000-000000000002"
    private val origin = "https://api.example.invalid"
    private fun partition(owner: String = a, org: String = a, server: String = origin) = EnterprisePartition.of(owner, server, org)

    private open class Memory : EnterprisePersistence {
        val records = mutableMapOf<Pair<EnterprisePartition,String>, EnterpriseLocalRecord>()
        val operations = linkedMapOf<Pair<EnterprisePartition,String>, EnterprisePendingOperation>()
        override suspend fun record(partition: EnterprisePartition, id: String) = records[partition to id]
        override suspend fun put(record: EnterpriseLocalRecord) { records[record.partition to record.id] = record }
        override suspend fun enqueue(operation: EnterprisePendingOperation): Boolean {
            val key = operation.partition to operation.id
            val previous = operations[key]
            if (previous != null) {
                if (previous.targetId != operation.targetId || !previous.envelope.contentEquals(operation.envelope)) throw EnterpriseOperationReused()
                return false
            }
            operations[key] = operation
            return true
        }
        override suspend fun pending(partition: EnterprisePartition, limit: Int) = operations.values.filter { it.partition == partition }.take(limit)
    }

    @Test
    fun `switching invalidates handles and work never follows the selected organization`(): Unit = runTest {
        val db = Memory()
        val context = EnterpriseLocalContext({ a }, { origin }, db)
        val old = context.select(partition())
        val queued = EnterprisePendingOperation(old.partition, a, b, byteArrayOf(1))
        old.put(EnterpriseLocalRecord(old.partition,b,byteArrayOf(2)))
        assertTrue(old.enqueue(queued))
        val other = context.select(partition(org = b))
        assertNull(other.record(b))
        assertTrue(other.pending().isEmpty())
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { old.pending() } }
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { other.enqueue(queued) } }
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { other.put(EnterpriseLocalRecord(partition(), b, byteArrayOf(3))) } }
        val restored = context.select(partition())
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { old.pending() } }
        assertEquals(partition(), restored.pending().single().partition)
        assertArrayEquals(byteArrayOf(1), restored.pending().single().envelope)
        assertArrayEquals(byteArrayOf(2), restored.record(b)!!.envelope)
        assertFalse(restored.enqueue(queued))
        assertThrows(EnterpriseOperationReused::class.java) { kotlinx.coroutines.runBlocking { restored.enqueue(EnterprisePendingOperation(partition(), a, b, byteArrayOf(8))) } }
    }

    @Test
    fun `wrong identity origin and logout refuse access and invalidate the old generation`(): Unit = runTest {
        var owner: String? = a
        var server = origin
        val context = EnterpriseLocalContext({ owner }, { server }, Memory())
        val first = context.select(partition())
        owner = b
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { first.pending() } }
        owner = a
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { first.pending() } }
        val second = context.select(partition())
        server = "https://other.example.invalid"
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { second.pending() } }
        server = origin
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { context.select(partition(owner=b)) } }
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { context.select(partition(server="https://other.example.invalid")) } }
        val third = context.select(partition())
        context.lock()
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { third.pending() } }
        owner = null
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { context.select(partition()) } }
        owner = a
        server = "http://127.0.0.1:18080"
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { context.select(partition()) } }
    }

    @Test
    fun `context changes serialize with an in flight call and stale replies are withheld`(): Unit = runTest {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        var owner = a
        val db = object : Memory() {
            override suspend fun record(partition: EnterprisePartition, id: String): EnterpriseLocalRecord? {
                entered.complete(Unit)
                release.await()
                return EnterpriseLocalRecord(partition,id,byteArrayOf(1))
            }
        }
        val context = EnterpriseLocalContext({owner},{origin},db)
        val store = context.select(partition())
        val read = async { runCatching { store.record(a) } }
        entered.await()
        val locking = async { context.lock() }
        runCurrent()
        assertFalse(locking.isCompleted)
        owner = b
        release.complete(Unit)
        assertTrue(read.await().exceptionOrNull() is EnterpriseLocalDenied)
        locking.await()
        owner = a
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { store.pending() } }
    }

    @Test
    fun `foreign or oversized persistence results fail closed`(): Unit = runTest {
        for (mode in listOf("record partition","record id","pending partition","pending size")) {
            val db = object : Memory() {
                override suspend fun record(partition: EnterprisePartition, id: String) =
                    EnterpriseLocalRecord(if(mode=="record partition") partition(org=b) else partition, if(mode=="record id") b else id, byteArrayOf(1))
                override suspend fun pending(partition: EnterprisePartition, limit: Int) =
                    if(mode=="pending size") List(limit+1){EnterprisePendingOperation(partition,a,b,byteArrayOf(1))}
                    else listOf(EnterprisePendingOperation(partition(org=b),a,b,byteArrayOf(1)))
            }
            val store = EnterpriseLocalContext({a},{origin},db).select(partition())
            assertThrows(EnterpriseLocalDenied::class.java) {
                kotlinx.coroutines.runBlocking { if(mode.startsWith("record")) store.record(a) else store.pending() }
            }
        }
    }

    @Test
    fun `production storage stays unavailable and restart requires fresh selection`(): Unit = runTest {
        val closed = EnterpriseLocalContext({a},{origin},UnavailableEnterprisePersistence).select(partition())
        assertThrows(EnterprisePersistenceUnavailable::class.java) { kotlinx.coroutines.runBlocking { closed.record(a) } }
        assertThrows(EnterprisePersistenceUnavailable::class.java) { kotlinx.coroutines.runBlocking { closed.put(EnterpriseLocalRecord(partition(),a,byteArrayOf())) } }
        assertThrows(EnterprisePersistenceUnavailable::class.java) { kotlinx.coroutines.runBlocking { closed.enqueue(EnterprisePendingOperation(partition(),a,b,byteArrayOf())) } }
        assertThrows(EnterprisePersistenceUnavailable::class.java) { kotlinx.coroutines.runBlocking { closed.pending() } }
        val db = Memory()
        val before = EnterpriseLocalContext({a},{origin},db).select(partition())
        before.enqueue(EnterprisePendingOperation(partition(),a,b,byteArrayOf(1)))
        val after = EnterpriseLocalContext({a},{origin},db).select(partition(org=b))
        assertTrue(after.pending().isEmpty())
        val original = EnterpriseLocalContext({a},{origin},db).select(partition())
        assertEquals(partition(),original.pending().single().partition)
        assertThrows(IllegalArgumentException::class.java) { kotlinx.coroutines.runBlocking { original.pending(0) } }
        assertThrows(IllegalArgumentException::class.java) { kotlinx.coroutines.runBlocking { original.pending(51) } }
        assertTrue(original.toString().contains("redacted"))
    }

    @Test
    fun `storage failure is redacted cancellation propagates and identity outages deny`(): Unit = runTest {
        var failure: Exception = IllegalStateException("PRIVATE-SQL-ROW")
        val db = object : Memory() {
            override suspend fun pending(partition: EnterprisePartition, limit: Int): List<EnterprisePendingOperation> = throw failure
        }
        val store = EnterpriseLocalContext({a},{origin},db).select(partition())
        val error = assertThrows(EnterprisePersistenceUnavailable::class.java) { kotlinx.coroutines.runBlocking { store.pending() } }
        assertNull(error.cause)
        assertFalse(error.toString().contains("PRIVATE"))
        failure = IllegalArgumentException("PRIVATE-SQL-ARGUMENT")
        val invalid = assertThrows(IllegalArgumentException::class.java) { kotlinx.coroutines.runBlocking { store.pending() } }
        assertFalse(invalid.toString().contains("PRIVATE"))
        failure = kotlinx.coroutines.CancellationException("synthetic cancellation")
        assertThrows(kotlinx.coroutines.CancellationException::class.java) { kotlinx.coroutines.runBlocking { store.pending() } }
        val outage = EnterpriseLocalContext({throw IllegalStateException("PRIVATE-SESSION")},{origin},db)
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { outage.select(partition()) } }
    }

    @Test
    fun `unlock and unrecoverable outcomes stay distinct from a storage outage`(): Unit = runTest {
        var failure: Exception = EnterpriseUnlockRequired()
        val db = object : Memory() {
            override suspend fun pending(partition: EnterprisePartition, limit: Int): List<EnterprisePendingOperation> = throw failure
        }
        val store = EnterpriseLocalContext({a},{origin},db).select(partition())
        assertThrows(EnterpriseUnlockRequired::class.java) { kotlinx.coroutines.runBlocking { store.pending() } }
        failure = EnterpriseVaultUnrecoverable()
        assertThrows(EnterpriseVaultUnrecoverable::class.java) { kotlinx.coroutines.runBlocking { store.pending() } }
        assertFalse(EnterpriseUnlockRequired().toString().contains(a))
    }
}
