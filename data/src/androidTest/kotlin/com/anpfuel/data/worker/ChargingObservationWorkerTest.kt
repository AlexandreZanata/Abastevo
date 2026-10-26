package com.anpfuel.data.worker

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.work.*
import androidx.work.testing.TestListenableWorkerBuilder
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.charging.ChargingCommunityUseCase
import com.anpfuel.data.remote.charging.ChargingObservationHttpClient
import com.anpfuel.domain.charging.*
import com.anpfuel.domain.portable.PortableAuth
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import org.junit.Assert.assertEquals
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class ChargingObservationWorkerTest {
    private val context:Context=ApplicationProvider.getApplicationContext()
    private fun worker(owner:String?="old-owner", attempt:Int=0, broken:Boolean=false):ChargingObservationWorker {
        val auth=object:AuthSessionStore {
            override fun load()=PortableAuth.Session("family","current-owner","synthetic","unused",Long.MAX_VALUE,Long.MAX_VALUE)
            override fun save(session:PortableAuth.Session)=Unit
            override fun clear()=Unit
        }
        val pending=object:ChargingPendingStore {
            override fun load(accountId:String, origin:String):List<PendingChargingOperation> {
                if(broken) throw java.io.IOException("synthetic storage outage")
                return emptyList()
            }
            override fun save(accountId:String,origin:String,operations:List<PendingChargingOperation>)=Unit
            override fun clear(accountId:String,origin:String)=Unit
        }
        // No call is possible without an eligible queued command. Loopback port 1
        // would fail the test rather than reach any public/staging service.
        val uc=ChargingCommunityUseCase(ChargingObservationHttpClient(OkHttpClient(),"http://127.0.0.1:1/"),pending,auth,"synthetic")
        return TestListenableWorkerBuilder<ChargingObservationWorker>(context).setRunAttemptCount(attempt)
            .setInputData(if(owner==null) Data.EMPTY else workDataOf("owner" to owner))
            .setWorkerFactory(object:WorkerFactory(){override fun createWorker(appContext:Context,workerClassName:String,params:WorkerParameters)=ChargingObservationWorker(appContext,params,uc)})
            .build()
    }
    @Test fun missingOwnerRefuses()=runBlocking {assertEquals(ListenableWorker.Result.failure(),worker(owner=null).doWork())}
    @Test fun formerOwnerStopsWithoutSendingNewOwnersQueue()=runBlocking {assertEquals(ListenableWorker.Result.success(),worker().doWork())}
    @Test fun storageOutageRetriesWithoutLosingCommandsAndStopsAtBudget()=runBlocking {
        assertEquals(ListenableWorker.Result.retry(),worker("current-owner",broken=true).doWork())
        assertEquals(ListenableWorker.Result.failure(),worker("current-owner",attempt=25,broken=true).doWork())
    }

    @Test fun enqueuesBehindInFlightChainWithoutLosingNewIntent() {
        androidx.work.testing.WorkManagerTestInitHelper.initializeTestWorkManager(context, Configuration.Builder().build())
        val scheduler=ChargingObservationScheduler(context)
        scheduler.enqueue("scheduler-test-owner");scheduler.enqueue("scheduler-test-owner")
        val infos=WorkManager.getInstance(context).getWorkInfosForUniqueWork("charging-observations-scheduler-test-owner").get()
        assertEquals(2,infos.size)
        org.junit.Assert.assertTrue(infos.all { it.constraints.requiredNetworkType == NetworkType.CONNECTED })
        WorkManager.getInstance(context).cancelUniqueWork("charging-observations-scheduler-test-owner").result.get()
    }
}
