package com.anpfuel.data.local

import androidx.room.Room
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.data.local.entity.*
import java.util.UUID
import kotlinx.coroutines.*
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/** Real Room cases compiled now, execution explicitly OWED until native batch. */
@RunWith(AndroidJUnit4::class)
class QualityReportOutboxDeviceTest {
    private fun database()=Room.inMemoryDatabaseBuilder(InstrumentationRegistry.getInstrumentation().targetContext,AnpFuelDatabase::class.java).build()
    private fun gate(key:String,local:String="synthetic-owner")=QualityOwnerGateEntity(key,local,"https://synthetic.invalid",false,100,1000)
    private fun row(key:String)=QualityReportOutboxEntity(key,UUID.randomUUID().toString(),"a".repeat(64),"synthetic-sealed",100,900,"QUEUED",0,100,"",0)
    @Test fun twelveClaimsHaveOneWinnerAndErasureFencesItsLateReceipt()=runBlocking {
        val db=database()
        try {
            val dao=db.qualityReportOutboxDao();val key="a".repeat(64)
            assertTrue(dao.activate(gate(key),100));val row=row(key);assertNotNull(dao.freeze(row,100))
            val start=CompletableDeferred<Unit>()
            val results=(1..12).map {async(Dispatchers.IO){start.await();dao.claim(key,row.commandId,UUID.randomUUID().toString(),200,300)}}
            start.complete(Unit)
            val winner=results.awaitAll().filterNotNull().single()
            assertEquals(1,winner.attempts)
            dao.erase("synthetic-owner",201,1000)
            assertFalse(dao.complete(key,row.commandId,winner.claimId,202))
            assertNull(dao.pending(key));assertTrue(dao.gate(key)!!.blocked)
        }finally {db.close()}
    }
    @Test fun concurrentAdmissionCannotEvictForeignRowsOrExceedPendingBudget()=runBlocking {
        val db=database()
        try {
            val dao=db.qualityReportOutboxDao()
            val owners=(1..32).map {"%064x".format(it)}
            owners.forEach {assertTrue(dao.activate(gate(it,"owner-$it"),100))}
            val start=CompletableDeferred<Unit>()
            val attempts=owners.map {key->async(Dispatchers.IO){start.await();dao.freeze(row(key),100)}}
            start.complete(Unit)
            assertEquals(25,attempts.awaitAll().count {it!=null})
            assertEquals(25,dao.pendingCount());assertEquals(32,dao.gatesCount())
        }finally {db.close()}
    }
}
