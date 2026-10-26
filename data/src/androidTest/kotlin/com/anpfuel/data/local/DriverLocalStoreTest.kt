package com.anpfuel.data.local

import androidx.room.Room
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.data.local.entity.DriverDraftEntity
import com.anpfuel.data.local.entity.DriverEarningEntity
import com.anpfuel.data.local.entity.DriverEarningRevisionEntity
import com.anpfuel.data.local.entity.DriverLocalCommandEntity
import com.anpfuel.data.local.entity.DriverShiftEntity
import com.anpfuel.data.local.entity.DriverShiftGapEntity
import com.anpfuel.data.repository.DriverLocalStore
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class DriverLocalStoreTest {
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext

    private fun open(name: String): AnpFuelDatabase =
        Room.databaseBuilder(context, AnpFuelDatabase::class.java, name).build()

    @Test fun draftsAreOwnerScopedRoundTripUnitsAndSurviveRestart() = runBlocking {
        val name = "driver-local-draft"
        context.deleteDatabase(name)
        var db = open(name)
        try {
            val store = DriverLocalStore(db.driverLocalDao())
            val draft = DriverDraftEntity(
                ownerAccount = "owner-a", origin = "app", draftId = "d1", kind = "REFUEL", recordId = "r1",
                bodyJson = """{"distance":"12,345","amount":"67,89"}""", distanceMetres = 12345,
                amountCentavos = 6789, durationSeconds = 90, unitVersion = "exact-v1", updatedAtMillis = 10,
            )
            store.saveDraft(draft)
            assertNull(store.findDraft("owner-b", "app", "d1"))
            assertNull(store.findDraft("", "app", "d1"))
            db.close()
            db = open(name)
            val reopened = DriverLocalStore(db.driverLocalDao()).findDraft("owner-a", "app", "d1")
            assertEquals(12345L, reopened?.distanceMetres)
            assertEquals(6789L, reopened?.amountCentavos)
            assertEquals(90L, reopened?.durationSeconds)
            assertEquals("exact-v1", reopened?.unitVersion)
            db.openHelper.readableDatabase.query(
                "SELECT name FROM pragma_table_info('driver_draft') UNION ALL SELECT name FROM pragma_table_info('driver_distance')",
            ).use { cursor ->
                while (cursor.moveToNext()) {
                    val column = cursor.getString(0).lowercase()
                    assertTrue(column, !column.contains("gps") && !column.contains("latitude") && !column.contains("longitude"))
                }
            }
        } finally {
            db.close()
            context.deleteDatabase(name)
        }
    }

    @Test fun earningRevisionAndCommandCommitOrRollbackTogether() = runBlocking {
        val name = "driver-local-atomic"
        context.deleteDatabase(name)
        val db = open(name)
        try {
            val store = DriverLocalStore(db.driverLocalDao())
            fun earning(net: Long, revision: Long, updated: Long) = DriverEarningEntity(
                ownerAccount = "owner-a", origin = "app", recordId = "r1", vehicleId = "v1", platform = "UBER",
                coverage = "SUMMARY", currency = "BRL", fromMillis = 1, toMillis = 2, timezone = "UTC", basis = "INCLUSIVE_NET",
                grossCentavos = null, feesCentavos = null, tipsCentavos = null, bonusCentavos = null, refundCentavos = null,
                netCentavos = net, revision = revision, contentSha256 = "a".repeat(64), updatedAtMillis = updated,
            )
            fun revision(rev: Long, created: Long) = DriverEarningRevisionEntity(
                ownerAccount = "owner-a", origin = "app", recordId = "r1", revision = rev,
                bodyJson = "{}", contentSha256 = "b".repeat(64), createdAtMillis = created,
            )
            fun command(op: String, updated: Long) = DriverLocalCommandEntity(
                ownerAccount = "owner-a", origin = "app", operationId = op, kind = "EARNING_SAVE",
                bodySha256 = "c".repeat(64), expectedRevision = 0, state = "QUEUED", attempts = 0,
                nextEligibleMillis = 0, lastError = null, updatedAtMillis = updated,
            )
            store.saveEarning(earning(100, 1, 1), revision(1, 1), command("op1", 1))
            var rolledBack = false
            try {
                store.saveEarning(earning(200, 1, 2), revision(1, 2), command("op2", 2))
            } catch (_: Throwable) {
                rolledBack = true
            }
            assertTrue("duplicate revision must fail", rolledBack)
            val current = store.findEarning("owner-a", "app", "r1")
            assertEquals(100L, current?.netCentavos)
            assertEquals(1, store.listEarningRevisions("owner-a", "app", "r1").size)
            assertNull(store.findCommand("owner-a", "app", "op2"))
            assertNull(store.findEarning("owner-b", "app", "r1"))
        } finally {
            db.close()
            context.deleteDatabase(name)
        }
    }

    @Test fun shiftGapReplacementIsAtomicAndOwnerScoped() = runBlocking {
        val name = "driver-local-shift"
        context.deleteDatabase(name)
        val db = open(name)
        try {
            val store = DriverLocalStore(db.driverLocalDao())
            val shift = DriverShiftEntity(
                ownerAccount = "owner-a", origin = "app", shiftId = "s1", vehicleId = "v1", timezone = "UTC",
                state = "RECOVERY_REQUIRED", version = 2, lastBootId = "b2", lastElapsedMillis = 2, lastUtcMillis = 2,
                activeMillis = 10, manuallyReviewedMillis = 0, createdAtMillis = 1, updatedAtMillis = 2,
            )
            fun gap(id: String) = DriverShiftGapEntity(
                ownerAccount = "owner-a", origin = "app", shiftId = "s1", gapId = id, reason = "boot-changed",
                previousBootId = "b1", previousElapsedMillis = 1, previousUtcMillis = 1,
                nextBootId = "b2", nextElapsedMillis = 2, nextUtcMillis = 2,
                reviewed = false, reviewedWorkMillis = null, createdAtMillis = 2,
            )
            store.saveShift(shift, listOf(gap("g1"), gap("g2")))
            assertEquals(2, store.listShiftGaps("owner-a", "app", "s1").size)
            store.saveShift(shift.copy(version = 3), listOf(gap("g2")))
            assertEquals(listOf("g2"), store.listShiftGaps("owner-a", "app", "s1").map { it.gapId })
            assertNull(store.findShift("owner-b", "app", "s1"))
            assertEquals(0, store.listShiftGaps("", "app", "s1").size)
        } finally {
            db.close()
            context.deleteDatabase(name)
        }
    }
}
