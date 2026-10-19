package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.QualityReportOutboxDao
import com.anpfuel.data.local.entity.*
import com.anpfuel.data.local.quality.*
import com.anpfuel.domain.quality.*
import com.anpfuel.domain.repository.QualityReportGateway
import io.mockk.*
import java.time.Instant
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import kotlinx.coroutines.*
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/** Orchestration tests; actual production SQL/concurrency is covered separately. */
class DurableQualityReportsTest {
    private class Fixture {
        var now=Instant.parse("2026-10-18T00:00:00Z").toEpochMilli()
        var owner:String?="https://teste.abastevo.com.br|account:synthetic:device|family:"+"a".repeat(64)
        var scheduled=0;var deleteFails=false
        val gateRows=mutableMapOf<String,QualityOwnerGateEntity>()
        val pendingRows=mutableMapOf<String,QualityReportOutboxEntity>()
        val secrets=mutableMapOf<String,SecretKey>()
        val deleted=mutableListOf<String>()
        val remote=mockk<QualityReportGateway>()
        val dao=mockk<QualityReportOutboxDao>(relaxed=true)
        val markers=MemoryQualityErasureMarkers()
        val keyOwners=mutableMapOf<String,String>()
        val keys=object:QualityReportKeys {
            override fun getOrCreate(ownerKey:String,local:String):SecretKey {keyOwners[ownerKey]=local;return secrets.getOrPut(ownerKey){KeyGenerator.getInstance("AES").apply {init(256)}.generateKey()}}
            override fun delete(ownerKey:String,local:String):Boolean {deleted+=ownerKey;if(deleteFails)return false;secrets.remove(ownerKey);return true}
            override fun eraseOwner(local:String):Boolean {if(deleteFails)return false;keyOwners.filterValues {it==local}.keys.forEach {delete(it,local)};return true}
        }
        init {
            every {remote.currentOwner()} answers {owner}
            coEvery {remote.owner()} coAnswers {owner ?: throw QualityReportException(QualityReportFailure.AUTH_REQUIRED)}
            coEvery {dao.activate(any(),any())} coAnswers {callOriginal()}
            coEvery {dao.freeze(any(),any())} coAnswers {callOriginal()}
            coEvery {dao.claim(any(),any(),any(),any(),any())} coAnswers {callOriginal()}
            coEvery {dao.complete(any(),any(),any(),any())} coAnswers {callOriginal()}
            coEvery {dao.failed(any(),any(),any(),any(),any())} coAnswers {callOriginal()}
            coEvery {dao.refused(any(),any(),any(),any())} coAnswers {callOriginal()}
            coEvery {dao.erase(any(),any(),any(),any())} coAnswers {callOriginal()}
            coEvery {dao.gate(any())} coAnswers {gateRows[firstArg()]}
            coEvery {dao.pending(any())} coAnswers {pendingRows[firstArg()]}
            coEvery {dao.gatesCount()} coAnswers {gateRows.size}
            coEvery {dao.pendingCount()} coAnswers {pendingRows.size}
            coEvery {dao.insertGate(any())} coAnswers {val g=firstArg<QualityOwnerGateEntity>();gateRows.putIfAbsent(g.ownerKey,g);1L}
            coEvery {dao.insertPending(any())} coAnswers {val r=firstArg<QualityReportOutboxEntity>();pendingRows[r.ownerKey]=r;Unit}
            coEvery {dao.touch(any(),any())} coAnswers {
                val key=firstArg<String>();val at=secondArg<Long>();val g=gateRows[key]
                if(g==null || g.blocked || g.keyErased || at<g.lastSeen || at>=g.expiresAt)0 else {gateRows[key]=g.copy(lastSeen=at);1}
            }
            coEvery {dao.claimRow(any(),any(),any(),any(),any())} coAnswers {
                val key=firstArg<String>();val r=pendingRows[key];val at=arg<Long>(3)
                if(r==null || r.commandId!=secondArg<String>() || r.attempts>=25 || r.retryAt>at || (r.state=="IN_FLIGHT" && r.leaseUntil>at))0
                else {pendingRows[key]=r.copy(state="IN_FLIGHT",attempts=r.attempts+1,claimId=arg(2),leaseUntil=arg(4));1}
            }
            coEvery {dao.acknowledge(any(),any(),any(),any())} coAnswers {if(pendingRows.remove(firstArg())!=null)1 else 0}
            coEvery {dao.failClaim(any(),any(),any(),any(),any())} coAnswers {
                val key=firstArg<String>();val r=pendingRows[key]
                if(r==null)0 else {pendingRows[key]=r.copy(state="FAILED",claimId="",leaseUntil=0,retryAt=arg(4));1}
            }
            coEvery {dao.refuseClaim(any(),any(),any())} coAnswers {if(pendingRows.remove(firstArg())!=null)1 else 0}
            coEvery {dao.blockOwner(any(),any(),any())} coAnswers {
                val local=firstArg<String>();gateRows.replaceAll {_,g->if(g.ownerLocal==local)g.copy(blocked=true,expiresAt=maxOf(g.expiresAt,arg(2))) else g};1
            }
            coEvery {dao.eraseRows(any())} coAnswers {
                val local=firstArg<String>();val owned=gateRows.values.filter {it.ownerLocal==local}.map {it.ownerKey}
                val count=pendingRows.keys.count {it in owned};pendingRows.keys.removeAll(owned.toSet());count
            }
            coEvery {dao.ownerKeys(any())} coAnswers {val local=firstArg<String>();gateRows.values.filter {it.ownerLocal==local}.map {it.ownerKey}}
            coEvery {dao.keysToErase(any())} coAnswers {val at=firstArg<Long>();gateRows.values.filter {!it.keyErased && (it.blocked || it.expiresAt<=at)}.map {it.ownerKey}}
            coEvery {dao.keyErased(any(),any())} coAnswers {val key=firstArg<String>();gateRows[key]=gateRows[key]!!.copy(keyErased=true);1}
            coEvery {remote.execute(any(),any())} coAnswers {val c=secondArg<QualityReportCommand>();QualityReportReceipt(c.reportId,c.expectedRevision+1,"PENDING_REVIEW")}
        }
        fun client(enabled:Boolean=true)=DurableQualityReports(remote,dao,keys,{scheduled++},{now},{enabled},markers)
        suspend fun start():Pair<DurableQualityReports,String> {val c=client();return c to c.owner()}
        fun command()=QualityReportCommand("eeeeeeee-5555-4555-8555-000000000001","eeeeeeee-5555-4555-8555-000000000002","SUBMIT",0,"EXPERIENCE","eeeeeeee-5555-4555-8555-000000000003","ETHANOL","2020-01-01","OTHER","Synthetic private experience")
    }
    @Test fun `uncertain send survives restart and reconnect with identical intent and fresh gateway call`()=runTest {
        val f=Fixture();val (first,owner)=f.start();val commands=mutableListOf<QualityReportCommand>()
        coEvery {f.remote.execute(owner,capture(commands))} coAnswers {
            if(commands.size==1)throw QualityReportException(QualityReportFailure.UNAVAILABLE)
            val c=secondArg<QualityReportCommand>();QualityReportReceipt(c.reportId,1,"PENDING_REVIEW")
        }
        try {first.execute(owner,f.command());fail<Unit>("uncertain outcome acknowledged") }catch(_:QualityReportException){}
        val frozen=f.pendingRows.values.single()
        assertFalse(frozen.sealedCommand.contains(f.command().text));assertEquals(1,frozen.attempts)
        val restarted=f.client();assertEquals(f.command(),restarted.pending(owner))
        f.now=frozen.retryAt
        assertTrue(restarted.runOnce())
        assertEquals(listOf(f.command(),f.command()),commands);assertTrue(f.pendingRows.isEmpty())
        assertTrue(f.scheduled>0)
    }
    @Test fun `server quota and exhausted attempts cannot spin or fork command`()=runTest {
        val f=Fixture();val (client,owner)=f.start()
        coEvery {f.remote.execute(any(),any())} throws QualityReportException(QualityReportFailure.QUOTA,3600)
        try {client.execute(owner,f.command())}catch(_:QualityReportException){}
        val frozen=f.pendingRows.values.single()
        assertEquals(f.now+3600000,frozen.retryAt)
        assertFalse(client.runOnce())
        coVerify(exactly=1){f.remote.execute(any(),any())}
        f.pendingRows[frozen.ownerKey]=frozen.copy(attempts=25,retryAt=Long.MAX_VALUE)
        assertTrue(client.runOnce());coVerify(exactly=1){f.remote.execute(any(),any())}
        try {client.execute(owner,f.command().copy(commandId="eeeeeeee-5555-4555-8555-000000000004"));fail<Unit>("competing intent replaced pending") }catch(e:QualityReportException){assertEquals(QualityReportFailure.CONFLICT,e.kind)}
    }
    @Test fun `logout during blocked response cannot retain receipt or recreate purged private state`()=runTest {
        val f=Fixture();val (client,owner)=f.start();val entered=CompletableDeferred<Unit>();val release=CompletableDeferred<Unit>()
        coEvery {f.remote.execute(any(),any())} coAnswers {entered.complete(Unit);release.await();QualityReportReceipt(f.command().reportId,1,"PENDING_REVIEW")}
        val send=async {try {client.execute(owner,f.command());false}catch(_:QualityReportException){true}}
        entered.await()
        assertTrue(client.eraseOwner("account:synthetic:device"));f.owner=null
        release.complete(Unit);assertTrue(send.await())
        assertTrue(f.pendingRows.isEmpty());assertTrue(f.gateRows.values.single().blocked)
        assertTrue(f.gateRows.values.single().keyErased);assertTrue(f.secrets.isEmpty())
    }
    @Test fun `failed key deletion retains durable repair obligation without touching active foreign key`()=runTest {
        val f=Fixture();val (client,owner)=f.start()
        coEvery {f.remote.execute(any(),any())} throws QualityReportException(QualityReportFailure.UNAVAILABLE)
        try {client.execute(owner,f.command())}catch(_:QualityReportException){}
        f.gateRows["b".repeat(64)]=QualityOwnerGateEntity("b".repeat(64),"foreign","https://foreign.invalid",false,f.now,f.now+100000)
        f.keys.getOrCreate("b".repeat(64),"foreign")
        f.deleteFails=true
        assertFalse(client.eraseOwner("account:synthetic:device"))
        assertFalse(f.gateRows.values.single {it.ownerLocal=="account:synthetic:device"}.keyErased)
        assertTrue(f.pendingRows.isEmpty())
        f.deleteFails=false
        assertTrue(client.repairKeys())
        assertTrue(f.gateRows.values.single {it.ownerLocal=="account:synthetic:device"}.keyErased)
        assertNotNull(f.secrets["b".repeat(64)])
    }
    @Test fun `different login family expired clock and disabled dispatch never send retained intent`()=runTest {
        val f=Fixture();val (client,owner)=f.start()
        coEvery {f.remote.execute(any(),any())} throws QualityReportException(QualityReportFailure.UNAVAILABLE)
        try {client.execute(owner,f.command())}catch(_:QualityReportException){}
        val original=f.pendingRows.values.single()
        assertTrue(f.client(false).runOnce())
        f.owner=owner.replace("family:"+"a".repeat(64),"family:"+"b".repeat(64))
        assertTrue(client.runOnce());coVerify(exactly=1){f.remote.execute(any(),any())}
        f.owner=owner;f.now=original.createdAt-1
        assertNull(client.pending(owner));assertFalse(client.runOnce())
        f.now=original.expiresAt
        assertTrue(client.runOnce());assertTrue(f.gateRows[original.ownerKey]!!.keyErased)
        coVerify(exactly=1){f.remote.execute(any(),any())}
    }
    @Test fun `intake switch pauses new claims but retains explicit owner withdrawal`()=runTest {
        val f=Fixture();val owner=f.client().owner();val paused=f.client(false)
        try {paused.execute(owner,f.command());fail<Unit>("disabled intake dispatched") }catch(e:QualityReportException){assertEquals(QualityReportFailure.UNAVAILABLE,e.kind)}
        assertTrue(f.pendingRows.isEmpty())
        val withdrawal=QualityReportCommand(f.command().commandId,f.command().reportId,"WITHDRAW",1)
        assertEquals(2,paused.execute(owner,withdrawal).revision)
        coVerify(exactly=1){f.remote.execute(owner,withdrawal)}
    }

    @Test fun `Room outage still erases owner keys and durable marker repairs after restart`()=runTest {
        val f=Fixture();val (client,owner)=f.start()
        coEvery {f.remote.execute(any(),any())} throws QualityReportException(QualityReportFailure.UNAVAILABLE)
        try {client.execute(owner,f.command())}catch(_:QualityReportException){}
        coEvery {f.dao.erase(any(),any(),any(),any())} throws IllegalStateException("synthetic database outage")
        assertFalse(client.eraseOwner("account:synthetic:device"))
        assertTrue(f.secrets.isEmpty());assertTrue(f.markers.blocked("account:synthetic:device"))
        assertNull(client.currentOwner())
        coEvery {f.dao.erase(any(),any(),any(),any())} coAnswers {callOriginal()}
        assertTrue(f.client().repairKeys())
        assertFalse(f.markers.blocked("account:synthetic:device"));assertTrue(f.pendingRows.isEmpty())
        assertTrue(f.gateRows.values.single().blocked)
    }

}
