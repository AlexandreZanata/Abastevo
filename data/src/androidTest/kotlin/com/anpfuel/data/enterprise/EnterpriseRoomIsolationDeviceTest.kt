package com.anpfuel.data.enterprise

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterprisePendingOperation
import java.util.UUID
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/** Synthetic native Room proof. Execution remains in the owner's deferred device batch. */
@RunWith(AndroidJUnit4::class)
class EnterpriseRoomIsolationDeviceTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private val name = "enterprise-synthetic-" + UUID.randomUUID() + ".db"
    private var db: EnterpriseDatabase? = null
    private val a = "7b0e2a54-0000-4000-8000-000000000001"
    private val b = "7b0e2a54-0000-4000-8000-000000000002"
    private val origin = "https://api.example.invalid"

    private fun open(): RoomEnterprisePersistence {
        db?.close()
        db = Room.databaseBuilder(context, EnterpriseDatabase::class.java, name).build()
        return RoomEnterprisePersistence(db!!.enterpriseDao())
    }

    @After fun cleanup() {
        db?.close()
        context.deleteDatabase(name)
    }

    @Test fun scoped_ids_and_immutable_pending_work_survive_reopening() = runBlocking {
        var store = open()
        val partitions = listOf(
            EnterprisePartition.of(a,origin,a), EnterprisePartition.of(b,origin,a),
            EnterprisePartition.of(a,"https://other.example.invalid",a), EnterprisePartition.of(a,origin,b),
        )
        for ((index,p) in partitions.withIndex()) {
            store.put(EnterpriseLocalRecord(p,a,byteArrayOf(index.toByte())))
            assertTrue(store.enqueue(EnterprisePendingOperation(p,a,b,byteArrayOf(index.toByte()))))
        }
        store = open()
        for ((index,p) in partitions.withIndex()) {
            assertArrayEquals(byteArrayOf(index.toByte()),store.record(p,a)!!.envelope)
            assertEquals(p,store.pending(p,50).single().partition)
            assertFalse(store.enqueue(EnterprisePendingOperation(p,a,b,byteArrayOf(index.toByte()))))
        }
        assertTrue(store.pending(EnterprisePartition.of(b,origin,b),50).isEmpty())
    }

    @Test fun concurrent_room_transactions_admit_one_operation() = runBlocking {
        val store = open()
        val p = EnterprisePartition.of(a,origin,a)
        val outcomes = List(12) {
            async(Dispatchers.IO) { store.enqueue(EnterprisePendingOperation(p,a,b,byteArrayOf(1))) }
        }.awaitAll()
        assertEquals(1,outcomes.count { it })
        assertEquals(1,store.pending(p,50).size)
        assertArrayEquals(byteArrayOf(1),store.pending(p,50).single().envelope)
    }
}
