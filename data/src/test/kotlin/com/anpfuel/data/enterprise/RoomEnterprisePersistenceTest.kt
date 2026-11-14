package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.EnterpriseOperationReused
import com.anpfuel.application.enterprise.EnterpriseLocalIdentity
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.data.di.EnterpriseModule
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterprisePendingOperation
import com.anpfuel.domain.portable.PortableAuth
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path

class RoomEnterprisePersistenceTest {
    @TempDir lateinit var directory: Path
    private val a = "7b0e2a54-0000-4000-8000-000000000001"
    private val b = "7b0e2a54-0000-4000-8000-000000000002"
    private val origin = "https://api.example.invalid"
    private fun partition(owner: String = a, server: String = origin, org: String = a) = EnterprisePartition.of(owner,server,org)

    private class Dao : EnterpriseDao() {
        private val records = mutableListOf<EnterpriseRecordRow>()
        private val operations = mutableListOf<EnterpriseOperationRow>()
        override suspend fun record(owner: String, origin: String, org: String, id: String) =
            records.find { it.ownerAccount==owner && it.origin==origin && it.organizationId==org && it.id==id }
        override suspend fun put(record: EnterpriseRecordRow) {
            records.removeAll { it.ownerAccount==record.ownerAccount && it.origin==record.origin && it.organizationId==record.organizationId && it.id==record.id }
            records += record
        }
        override suspend fun operation(owner: String, origin: String, org: String, id: String) =
            operations.find { it.ownerAccount==owner && it.origin==origin && it.organizationId==org && it.id==id }
        override suspend fun insertOperation(operation: EnterpriseOperationRow): Long {
            if(operation(operation.ownerAccount,operation.origin,operation.organizationId,operation.id)!=null) return -1
            operations += operation
            return operations.size.toLong()
        }
        override suspend fun pending(owner: String, origin: String, org: String, limit: Int) =
            operations.filter { it.ownerAccount==owner && it.origin==origin && it.organizationId==org }.take(limit)
    }

    @Test
    fun `colliding ids across account origin and organization remain isolated`(): Unit = runBlocking {
        val dao = Dao()
        val store = RoomEnterprisePersistence(dao)
        val partitions = listOf(partition(),partition(owner=b),partition(server="https://other.example.invalid"),partition(org=b))
        for ((index,p) in partitions.withIndex()) {
            store.put(EnterpriseLocalRecord(p,a,byteArrayOf(index.toByte())))
            assertTrue(store.enqueue(EnterprisePendingOperation(p,b,a,byteArrayOf(index.toByte()))))
        }
        for ((index,p) in partitions.withIndex()) {
            assertArrayEquals(byteArrayOf(index.toByte()),store.record(p,a)!!.envelope)
            assertNull(store.record(p,b))
            assertEquals(p,store.pending(p,50).single().partition)
            assertArrayEquals(byteArrayOf(index.toByte()),store.pending(p,50).single().envelope)
        }
        // Reconstruct the adapter as a process-restored dependency graph.
        val reopened = RoomEnterprisePersistence(dao)
        assertEquals(partition(),reopened.pending(partition(),1).single().partition)
        assertThrows(IllegalArgumentException::class.java) { runBlocking { reopened.pending(partition(),0) } }
        assertThrows(IllegalArgumentException::class.java) { runBlocking { reopened.pending(partition(),51) } }
    }

    @Test
    fun `duplicate operation converges but changed intent never overwrites held work`(): Unit = runBlocking {
        val store = RoomEnterprisePersistence(Dao())
        val op = EnterprisePendingOperation(partition(),a,b,byteArrayOf(4))
        assertTrue(store.enqueue(op))
        assertFalse(store.enqueue(EnterprisePendingOperation(partition(),a,b,byteArrayOf(4))))
        assertThrows(EnterpriseOperationReused::class.java) { runBlocking { store.enqueue(EnterprisePendingOperation(partition(),a,a,byteArrayOf(4))) } }
        assertThrows(EnterpriseOperationReused::class.java) { runBlocking { store.enqueue(EnterprisePendingOperation(partition(),a,b,byteArrayOf(5))) } }
        val retained = store.pending(partition(),1).single()
        assertEquals(b,retained.targetId)
        assertArrayEquals(byteArrayOf(4),retained.envelope)
        for(row in listOf(EnterpriseRecordRow(a,origin,a,b,byteArrayOf(4)),EnterpriseOperationRow(a,origin,a,b,a,byteArrayOf(4)))){
            assertFalse(row.toString().contains(a))
            assertFalse(row.toString().contains(origin))
        }
    }

    @Test
    fun `production graph retains protected custody and current account origin and shared fence`(): Unit = runBlocking {
        val fixture = VaultFixture(directory)
        fixture.identity = EnterpriseLocalIdentity(a, ApiEnvironment.STAGING.origin, a)
        val persistence = EnterpriseModule.persistence(fixture.store)
        val auth = mockk<AuthFlow>()
        val session = PortableAuth.Session(a,a,"synthetic-access","synthetic-refresh",100,200)
        every { auth.rehydrate() } returns AuthFlow.AuthState.Active(session)
        val fence = fixture.fence
        val context = EnterpriseModule.localContext(auth,ApiEnvironment.STAGING,persistence,fence)
        val p = EnterprisePartition.of(a,ApiEnvironment.STAGING.origin,b)
        val unconfirmed = context.select(p)
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { unconfirmed.pending() } }
        fixture.confirm(p)
        val confirmed = context.select(p)
        assertTrue(confirmed.pending().isEmpty())
        assertTrue(confirmed.enqueue(EnterprisePendingOperation(p, a, b, byteArrayOf(7))))
        // The graph's shared fence, not a private one, invalidates selected handles.
        val backgrounded = context.select(p)
        fence.markBackground()
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { backgrounded.pending() } }
        val bound = context.select(p)
        assertEquals(1, bound.pending().size)
        val contacts = fixture.contacts
        every { auth.rehydrate() } returns AuthFlow.AuthState.NeedsRefresh(session)
        assertEquals(1, bound.pending().size)
        assertEquals(contacts, fixture.contacts)
        every { auth.rehydrate() } returns AuthFlow.AuthState.LoggedOut
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { bound.pending() } }
        every { auth.rehydrate() } returns AuthFlow.AuthState.Active(session.copy(accountId=b))
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { context.select(p) } }
        every { auth.rehydrate() } returns AuthFlow.AuthState.Active(session)
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { context.select(partition()) } }
        // Debug loopback configuration must not crash the existing app graph.
        val loopback = EnterpriseModule.localContext(auth,ApiEnvironment.LOCAL_PROOF,persistence,fence)
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { loopback.select(p) } }
    }
}
