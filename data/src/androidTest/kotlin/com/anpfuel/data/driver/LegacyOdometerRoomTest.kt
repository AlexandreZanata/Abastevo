package com.anpfuel.data.driver

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.dao.LedgerDraftDao
import com.anpfuel.data.local.entity.LedgerDraftEntity
import com.anpfuel.data.repository.LedgerDraftStoreImpl
import com.anpfuel.domain.premium.*
import java.io.IOException
import java.time.Instant
import java.util.UUID
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.*
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/** Actual Room/SQLite persistence, CAS and restart. Synthetic private test database only. */
@RunWith(AndroidJUnit4::class)
class LegacyOdometerRoomTest {
    private lateinit var db: AnpFuelDatabase
    private lateinit var context: Context
    private lateinit var name: String
    private val origin="https://local.example.test"
    private val fill=RefuelDraft("11111111-1111-4111-8111-111111111111","22222222-2222-4222-8222-222222222222",Instant.parse("2026-03-01T12:00:00Z"),"UTC",PremiumFuel.ETHANOL,40000,26000,null,10000,LedgerFill.FULL,null,"current note")
    private fun open() = Room.databaseBuilder(context,AnpFuelDatabase::class.java,name).build()
    @Before fun prepare() { context=ApplicationProvider.getApplicationContext();name="drv02-"+UUID.randomUUID()+".db";db=open() }
    @After fun finish() { db.close();context.deleteDatabase(name) }
    private suspend fun seed(): LedgerDraftEntity = LedgerDraftEntity("legacy","a",origin,"refuel",fill.id,2,fill.bodyJson(),1,"home").also { db.ledgerDraftDao().upsert(it) }
    private suspend fun expect(code:String, work:suspend()->Unit) {
        try { work(); fail("expected refusal") } catch(e:PremiumValidationException) { assertEquals(code,e.code) }
    }

    @Test fun legacy_review_restart_and_command_keep_money_and_original_numeric_receipt() = runBlocking {
        seed()
        val store=LedgerDraftStoreImpl(db.ledgerDraftDao())
        assertEquals(OdometerEvidence.UNVERIFIED,store.find("a",origin,fill.dataset,fill.id)!!.odometerEvidence)
        val hash=LegacyOdometerReview.fingerprint(fill)
        val reviewed=store.reviewOdometer("a",origin,fill.dataset,fill.id,hash,OdometerReviewChoice.AS_KILOMETRES,10)
        assertEquals(10000000L,(reviewed.draft as RefuelDraft).odometerMetres)
        assertEquals(26000L,(reviewed.draft as RefuelDraft).totalCentavos)
        assertEquals(LedgerSessionContext.HOME,reviewed.context)
        val raw=db.ledgerDraftDao().findByRecord("a",origin,"refuel",fill.id)!!
        val receipt=JSONObject(raw.bodyJson).getJSONObject("_abastevo_odometer").getJSONObject("review")
        assertEquals(10000L,receipt.getLong("original"))
        assertFalse(receipt.toString().contains("current note"))
        val body=LedgerCommandFactory.toCommand(PremiumOwnerScope("a",origin),reviewed.draft,reviewed.expectedRevision).payloadJson
        assertFalse(body.contains("_abastevo_odometer"))
        assertTrue(body.contains("\"odometer_metres\":10000000"))
        db.close();db=open()
        val reloaded=LedgerDraftStoreImpl(db.ledgerDraftDao())
        assertEquals(OdometerEvidence.VERIFIED_METRES,reloaded.find("a",origin,fill.dataset,fill.id)!!.odometerEvidence)
        assertEquals(10L,reloaded.reviewOdometer("a",origin,fill.dataset,fill.id,hash,OdometerReviewChoice.AS_KILOMETRES,99).updatedAtMillis)
        expect("draft-missing") { reloaded.reviewOdometer("b",origin,fill.dataset,fill.id,hash,OdometerReviewChoice.AS_KILOMETRES,100) }
        expect("draft-missing") { reloaded.reviewOdometer("a","https://other.example.test",fill.dataset,fill.id,hash,OdometerReviewChoice.AS_KILOMETRES,100) }
        reloaded.clearOwner("a",origin)
        assertNull(reloaded.find("a",origin,fill.dataset,fill.id))
    }

    private fun rendezvous(real:LedgerDraftDao): LedgerDraftDao {
        val reads=AtomicInteger();val gate=CompletableDeferred<Unit>()
        return object: LedgerDraftDao by real {
            override suspend fun findByRecord(owner:String,origin:String,dataset:String,recordId:String):LedgerDraftEntity? {
                val row=real.findByRecord(owner,origin,dataset,recordId)
                val n=reads.incrementAndGet()
                if(n<=2) { if(n==2) gate.complete(Unit);gate.await() }
                return row
            }
        }
    }

    @Test fun concurrent_identical_reviews_converge_without_multiplying_twice() = runBlocking {
        seed();val store=LedgerDraftStoreImpl(rendezvous(db.ledgerDraftDao()))
        val hash=LegacyOdometerReview.fingerprint(fill)
        val results=listOf(1,2).map { async(Dispatchers.Default) { store.reviewOdometer("a",origin,fill.dataset,fill.id,hash,OdometerReviewChoice.AS_KILOMETRES,it.toLong()) } }.awaitAll()
        assertEquals(results[0],results[1])
        assertEquals(10000000L,(results[0].draft as RefuelDraft).odometerMetres)
    }

    @Test fun concurrent_different_choices_have_one_winner_and_one_visible_conflict() = runBlocking {
        seed();val store=LedgerDraftStoreImpl(rendezvous(db.ledgerDraftDao()));val hash=LegacyOdometerReview.fingerprint(fill)
        val results=OdometerReviewChoice.entries.map { choice -> async(Dispatchers.Default) { runCatching { store.reviewOdometer("a",origin,fill.dataset,fill.id,hash,choice,10) } } }.awaitAll()
        assertEquals(1,results.count { it.isSuccess })
        assertEquals("stale-odometer-review",(results.single { it.isFailure }.exceptionOrNull() as PremiumValidationException).code)
        val final=LedgerDraftStoreImpl(db.ledgerDraftDao()).find("a",origin,fill.dataset,fill.id)!!
        assertEquals(results.single { it.isSuccess }.getOrThrow(),final)
    }

    @Test fun late_edit_between_preview_and_commit_survives_the_refused_review() = runBlocking {
        val row=seed();val real=db.ledgerDraftDao()
        val intercepted=object:LedgerDraftDao by real {
            override suspend fun compareAndSetBody(owner:String,origin:String,dataset:String,recordId:String,expectedRevision:Long,expectedBody:String,replacementBody:String,nowMillis:Long,draftId:String):Int {
                real.upsert(row.copy(bodyJson=fill.copy(totalCentavos=27000).bodyJson()))
                return real.compareAndSetBody(owner,origin,dataset,recordId,expectedRevision,expectedBody,replacementBody,nowMillis,draftId)
            }
        }
        expect("stale-odometer-review") { LedgerDraftStoreImpl(intercepted).reviewOdometer("a",origin,fill.dataset,fill.id,LegacyOdometerReview.fingerprint(fill),OdometerReviewChoice.AS_KILOMETRES,10) }
        val final=LedgerDraftStoreImpl(real).find("a",origin,fill.dataset,fill.id)!!
        assertEquals(27000L,(final.draft as RefuelDraft).totalCentavos)
        assertEquals(10000L,(final.draft as RefuelDraft).odometerMetres)
        assertEquals(OdometerEvidence.UNVERIFIED,final.odometerEvidence)
    }

    @Test fun cancellation_before_write_does_not_mutate_and_lost_ack_replays_after_commit() = runBlocking {
        val raw=seed();val real=db.ledgerDraftDao();val hash=LegacyOdometerReview.fingerprint(fill)
        val cancelled=object:LedgerDraftDao by real {
            override suspend fun compareAndSetBody(owner:String,origin:String,dataset:String,recordId:String,expectedRevision:Long,expectedBody:String,replacementBody:String,nowMillis:Long,draftId:String):Int = throw CancellationException("synthetic cancellation")
        }
        try { LedgerDraftStoreImpl(cancelled).reviewOdometer("a",origin,fill.dataset,fill.id,hash,OdometerReviewChoice.AS_KILOMETRES,10);fail("cancel accepted") } catch(_:CancellationException) { }
        assertEquals(raw.bodyJson,real.findByRecord("a",origin,"refuel",fill.id)!!.bodyJson)
        val lost=object:LedgerDraftDao by real {
            override suspend fun compareAndSetBody(owner:String,origin:String,dataset:String,recordId:String,expectedRevision:Long,expectedBody:String,replacementBody:String,nowMillis:Long,draftId:String):Int {
                real.compareAndSetBody(owner,origin,dataset,recordId,expectedRevision,expectedBody,replacementBody,nowMillis,draftId)
                throw IOException("synthetic lost acknowledgment")
            }
        }
        try { LedgerDraftStoreImpl(lost).reviewOdometer("a",origin,fill.dataset,fill.id,hash,OdometerReviewChoice.AS_KILOMETRES,10);fail("lost ack accepted") } catch(_:IOException) { }
        val accepted=LedgerDraftStoreImpl(real).reviewOdometer("a",origin,fill.dataset,fill.id,hash,OdometerReviewChoice.AS_KILOMETRES,99)
        assertEquals(10000000L,(accepted.draft as RefuelDraft).odometerMetres)
        assertEquals(10L,accepted.updatedAtMillis)
    }

    @Test fun fresh_proof_persists_and_legacy_rows_are_not_mass_converted() = runBlocking {
        seed();val store=LedgerDraftStoreImpl(db.ledgerDraftDao())
        val fresh=fill.copy(id="33333333-3333-4333-8333-333333333333",odometerMetres=12345600)
        store.save("a",origin,fresh,0,10,null,OdometerEvidence.VERIFIED_METRES)
        assertEquals(OdometerEvidence.VERIFIED_METRES,store.find("a",origin,fresh.dataset,fresh.id)!!.odometerEvidence)
        assertEquals(10000L,(store.find("a",origin,fill.dataset,fill.id)!!.draft as RefuelDraft).odometerMetres)
        assertEquals(OdometerEvidence.UNVERIFIED,store.find("a",origin,fill.dataset,fill.id)!!.odometerEvidence)
        store.delete("a",origin,fresh.dataset,fresh.id)
        assertEquals(1,store.list("a",origin).size)
    }
}
