package com.anpfuel.data.repository

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.domain.model.ContributionDraft
import com.anpfuel.domain.repository.ContributionReceipt
import com.anpfuel.domain.repository.ContributionRemoteStatus
import com.anpfuel.domain.repository.PendingContribution
import com.anpfuel.domain.valueobject.FuelProduct
import java.util.UUID
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/** App-owned synthetic SQLite only; never dispatches a photo or modifies main app data. */
@RunWith(AndroidJUnit4::class)
class PhotoReviewOutboxDeviceTest {
    private fun pending(id: String, fuel: FuelProduct = FuelProduct.ETHANOL) = PendingContribution(
        ContributionDraft.create(id, "d6c74c23-63db-4c24-a2e5-408cb23bad26", fuel,
            if (fuel == FuelProduct.GASOLINE_PREMIUM_GRADE) 9190 else 3990, "BRL", "L", "STANDARD", 1_900_000L),
        org.json.JSONObject().put("client_submission_id",id).put("freshness","fresh").put("fuel_product",fuel.name).put("amount_milli_brl",if (fuel == FuelProduct.GASOLINE_PREMIUM_GRADE) 9190 else 3990).toString(),
    )

    @Test fun frozenBatchRollbackAndConcurrentClaimUseActualRoom() = runBlocking {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val db = Room.inMemoryDatabaseBuilder(context, AnpFuelDatabase::class.java).build()
        try {
            val dao = db.contributionOutboxDao()
            val repo = RoomContributionOutboxRepository(dao)
            val first = repo.enqueueReview(listOf(pending("one"), pending("two")))
            assertEquals(first, repo.enqueueReview(listOf(pending("one"), pending("two"))))
            val saved = dao.findById("one")!!
            try {
                dao.insertReview(listOf(saved.copy(commandId = "new"), saved))
                fail("duplicate insert did not abort")
            } catch (_: android.database.sqlite.SQLiteConstraintException) { }
            assertNull(dao.findById("new"))
            assertEquals(2, dao.listAll().size)
            val claims = coroutineScope {
                (1..8).map { index -> async { repo.claimDispatch("one", 1, "attempt-$index", 2_000_000L) } }.awaitAll()
            }
            assertEquals(1, claims.count { it })
            assertFalse(repo.listDispatchable(2_000_001L).any { it.commandId == "one" })
            assertTrue(repo.listDispatchable(2_360_000L).any { it.commandId == "one" })
        } finally { db.close() }
    }

    @Test fun receivedReceiptAndCancelledSiblingSurviveReopeningDatabase() = runBlocking {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val name = "photo-review-owned-test-${UUID.randomUUID()}"
        fun open() = Room.databaseBuilder(context, AnpFuelDatabase::class.java, name).build()
        var db = open()
        try {
            var repo = RoomContributionOutboxRepository(db.contributionOutboxDao())
            repo.enqueueReview(listOf(pending("one"), pending("two")))
            repo.recordReceipt(ContributionReceipt("one", 1, ContributionRemoteStatus.RECEIVED,
                "e0000000-0000-4000-8000-000000000001"), 2_000_000L)
            val uploads = db.photoUploadSessionDao()
            val session = com.anpfuel.data.local.entity.PhotoUploadSessionEntity("c0000000-0000-4000-8000-000000000001",
                "e0000000-0000-4000-8000-000000000001","synthetic-photo","synthetic-owner","https://example.invalid",
                1_900_000,88_300_000,"a".repeat(64))
            uploads.remember(session)
            uploads.ready(session.captureId,session.sessionId,"e0000000-0000-4000-8000-000000000002")
            repo.cancel("two")
            db.close(); db = open(); repo = RoomContributionOutboxRepository(db.contributionOutboxDao())
            assertEquals(ContributionRemoteStatus.RECEIVED, repo.receipt("one")!!.status)
            assertEquals("e0000000-0000-4000-8000-000000000002",db.photoUploadSessionDao().find(session.captureId)!!.evidenceId)
            assertEquals(0,db.photoUploadSessionDao().ready(session.captureId,session.sessionId,"e0000000-0000-4000-8000-000000000003"))
            assertEquals(2, repo.listOwned().size)
            assertFalse(repo.listDispatchable(2_000_001L).any { it.commandId == "two" })
            assertEquals("one", repo.loadPayload("one")?.let { org.json.JSONObject(it).getString("client_submission_id") })
        } finally { db.close(); context.deleteDatabase(name) }
    }
    @Test fun premiumProductAndReceiptSurviveRoomReopening() = runBlocking {
        val context=ApplicationProvider.getApplicationContext<Context>()
        val name="premium-owned-test-${UUID.randomUUID()}"
        fun open()=Room.databaseBuilder(context,AnpFuelDatabase::class.java,name).build()
        var db=open()
        try {
            var repo=RoomContributionOutboxRepository(db.contributionOutboxDao())
            repo.enqueueReview(listOf(pending("premium",FuelProduct.GASOLINE_PREMIUM_GRADE)))
            repo.recordReceipt(ContributionReceipt("premium",1,ContributionRemoteStatus.RECEIVED,"e0000000-0000-4000-8000-000000000001"),2_000_000L)
            db.close();db=open();repo=RoomContributionOutboxRepository(db.contributionOutboxDao())
            val frozen=org.json.JSONObject(repo.loadPayload("premium")!!)
            assertEquals("GASOLINE_PREMIUM_GRADE",frozen.getString("fuel_product"))
            assertEquals(9190L,frozen.getLong("amount_milli_brl"))
            assertEquals(ContributionRemoteStatus.RECEIVED,repo.receipt("premium")!!.status)
        } finally {db.close();context.deleteDatabase(name)}
    }

}
