package com.anpfuel.data.local

import androidx.room.Room
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.data.local.entity.DriverLocalCommandEntity
import com.anpfuel.data.repository.DriverLocalStore
import com.anpfuel.data.repository.DriverOutbox
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class DriverOutboxTest {
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext

    private fun open(name: String): AnpFuelDatabase =
        Room.databaseBuilder(context, AnpFuelDatabase::class.java, name).build()

    private fun command(op: String, state: String = "PENDING", next: Long = 10) = DriverLocalCommandEntity(
        ownerAccount = "owner-a", origin = "app", operationId = op, kind = "EARNING_SAVE",
        bodySha256 = "a".repeat(64), expectedRevision = 0, state = state, attempts = 0,
        nextEligibleMillis = next, lastError = null, updatedAtMillis = 10,
    )

    @Test fun claimIsOwnerBoundLeaseAwareAndSurvivesRestart() = runBlocking {
        val name = "driver-outbox"
        context.deleteDatabase(name)
        var db = open(name)
        try {
            val store = DriverLocalStore(db.driverLocalDao())
            val queue = DriverOutbox(db.driverLocalDao())
            store.saveEarning(
                com.anpfuel.data.local.entity.DriverEarningEntity(
                    "owner-a", "app", "r1", "v1", "UBER", "SUMMARY", "BRL", 1, 2, "UTC", "INCLUSIVE_NET",
                    null, null, null, null, null, 100, 1, "a".repeat(64), 10,
                ),
                com.anpfuel.data.local.entity.DriverEarningRevisionEntity("owner-a", "app", "r1", 1, "{}", "b".repeat(64), 10),
                command("op1"),
            )
            assertNull(queue.claim("owner-b", "app", "op1", 100))
            assertNull(queue.claim("", "app", "op1", 100))
            assertNotNull(queue.claim("owner-a", "app", "op1", 100, 60))
            assertNull(queue.claim("owner-a", "app", "op1", 100, 60))
            assertNotNull(queue.claim("owner-a", "app", "op1", 200, 60))
            assertEquals(1, queue.retry("owner-a", "app", "op1", "offline", 300, 100, 1000))
            assertNull(queue.claim("owner-a", "app", "op1", 399, 60))
            assertNotNull(queue.claim("owner-a", "app", "op1", 400, 60))
            assertEquals(1, queue.synced("owner-a", "app", "op1", 500))
            assertEquals("SYNCED", store.findCommand("owner-a", "app", "op1")?.state)
            assertNull(queue.claim("owner-a", "app", "op1", 600, 60))
            db.close()
            db = open(name)
            val reopened = DriverOutbox(db.driverLocalDao())
            assertNull(reopened.claim("owner-a", "app", "op1", 700, 60))
        } finally {
            db.close()
            context.deleteDatabase(name)
        }
    }

    @Test fun conflictAndRetryLimitParkWithoutClaim() = runBlocking {
        val name = "driver-outbox-conflict"
        context.deleteDatabase(name)
        val db = open(name)
        try {
            val store = DriverLocalStore(db.driverLocalDao())
            val queue = DriverOutbox(db.driverLocalDao())
            store.saveEarning(
                com.anpfuel.data.local.entity.DriverEarningEntity(
                    "owner-a", "app", "r1", "v1", "UBER", "SUMMARY", "BRL", 1, 2, "UTC", "INCLUSIVE_NET",
                    null, null, null, null, null, 100, 1, "a".repeat(64), 10,
                ),
                com.anpfuel.data.local.entity.DriverEarningRevisionEntity("owner-a", "app", "r1", 1, "{}", "b".repeat(64), 10),
                command("op1"),
            )
            assertEquals(1, queue.conflict("owner-a", "app", "op1", "server-conflict", 100))
            assertEquals("CONFLICT", store.findCommand("owner-a", "app", "op1")?.state)
            assertNull(queue.claim("owner-a", "app", "op1", 200))
            store.saveEarning(
                com.anpfuel.data.local.entity.DriverEarningEntity(
                    "owner-a", "app", "r2", "v1", "UBER", "SUMMARY", "BRL", 1, 2, "UTC", "INCLUSIVE_NET",
                    null, null, null, null, null, 100, 1, "c".repeat(64), 10,
                ),
                com.anpfuel.data.local.entity.DriverEarningRevisionEntity("owner-a", "app", "r2", 1, "{}", "d".repeat(64), 10),
                command("op2"),
            )
            repeat(5) { queue.retry("owner-a", "app", "op2", "offline", 100, 10, 1000, 6) }
            assertEquals(1, queue.retry("owner-a", "app", "op2", "offline", 100, 10, 1000, 6))
            assertEquals("CONFLICT", store.findCommand("owner-a", "app", "op2")?.state)
            assertNull(queue.claim("owner-a", "app", "op2", 10_000))
        } finally {
            db.close()
            context.deleteDatabase(name)
        }
    }
}
