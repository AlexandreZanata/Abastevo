package com.anpfuel.data.remote.quality

import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.data.remote.PhotoProofTransport
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.quality.*
import io.mockk.*
import java.time.Clock
import java.time.Instant
import java.time.ZoneOffset
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class QualityReportHttpClientTest {
    private val id="eeeeeeee-5555-4555-8555-000000000001"
    private val station="eeeeeeee-5555-4555-8555-000000000002"
    private val clock=Clock.fixed(Instant.parse("2026-10-18T00:00:00Z"),ZoneOffset.UTC)
    private val owner="https://teste.abastevo.com.br|account:synthetic:device"
    private fun command()=QualityReportCommand(id,id,"SUBMIT",0,"EXPERIENCE",station,"ETHANOL","2026-10-18","OTHER","Synthetic claim")
    private fun receipt()=JSONObject().put("report_id",id).put("revision",1).put("status","PENDING_REVIEW")
    private fun report()=JSONObject().put("id",id).put("revision",1).put("kind","EXPERIENCE").put("station_id",station)
        .put("fuel","ETHANOL").put("occurred_on","2026-10-18").put("symptom","OTHER").put("text","Synthetic claim")
        .put("status","FUTURE_STATUS").put("evidence_level","USER_CLAIM")

    @Test fun `invalid command never reaches authority and foreign owner never reaches request`()=runTest {
        var sends=0
        val api=QualityReportHttpClient({owner},{owner},{_,_,_,_->sends++;receipt()},clock)
        for(c in listOf(command().copy(occurredOn="2027-01-01"),command().copy(text=""))) {
            val e=assertThrows(QualityReportException::class.java){runBlocking {api.execute(owner,c)}}
            assertEquals(QualityReportFailure.INVALID,e.kind)
        }
        assertThrows(QualityReportException::class.java){runBlocking {api.execute("other",command())}}
        assertEquals(0,sends)
    }
    @Test fun `owner change during response drops all private data`()=runTest {
        var current:String?=owner
        val api=QualityReportHttpClient({owner},{current},{_,_,_,_->current="other";report()},clock)
        val e=assertThrows(QualityReportException::class.java){runBlocking {api.get(owner,id)}}
        assertEquals(QualityReportFailure.OWNER_CHANGED,e.kind)
    }
    @Test fun `owner list is bounded ordered and cannot repeat cursor`()=runTest {
        var response=JSONObject().put("items",org.json.JSONArray().put(report()))
        val api=QualityReportHttpClient({owner},{owner},{_,_,_,_->response},clock)
        assertEquals("FUTURE_STATUS",api.list(owner).items.single().status)
        response=JSONObject().put("items",JSONObject.NULL)
        assertTrue(api.list(owner).items.isEmpty())
        for(bad in listOf(JSONObject().put("items",org.json.JSONArray().put(report()).put(report())),JSONObject().put("items",org.json.JSONArray()).put("next_after_id",id))) {
            response=bad
            assertThrows(QualityReportException::class.java){runBlocking {api.list(owner)}}
        }
        response=JSONObject().put("items",org.json.JSONArray().put(report()))
        assertThrows(QualityReportException::class.java){runBlocking {api.list(owner,id)}}
    }
    @Test fun `receipt mismatches and raw transport errors remain fixed failures`()=runTest {
        var response=receipt().put("report_id",station)
        val api=QualityReportHttpClient({owner},{owner},{_,_,_,_->response},clock)
        assertThrows(QualityReportException::class.java){runBlocking {api.execute(owner,command())}}
        response=receipt().put("revision",2)
        assertThrows(QualityReportException::class.java){runBlocking {api.execute(owner,command())}}
        val broken=QualityReportHttpClient({owner},{owner},{_,_,_,_->throw java.io.IOException("private upstream body")},clock)
        val e=assertThrows(QualityReportException::class.java){runBlocking {broken.get(owner,id)}}
        assertFalse(e.message.orEmpty().contains("private"))
    }
    @Test fun `retry keeps command but refreshes session and exact signed body`()=runTest {
        val proof=mockk<PhotoProofTransport>()
        val auth=mockk<AuthFlow>()
        val origin="https://teste.abastevo.com.br"
        val local="account:synthetic:device"
        val session=PortableAuth.Session(id,"synthetic","synthetic-access-1","synthetic-refresh",Instant.now().epochSecond+3600,Instant.now().epochSecond+7200)
        every {auth.rehydrate()} returns AuthFlow.AuthState.Active(session)
        every {proof.origin} returns origin
        every {proof.ownerScope()} returns local
        coEvery {proof.localScope()} returns local
        every {auth.refreshSession()} returnsMany listOf(AuthApiResult.Ok(session),AuthApiResult.Ok(session.copy(accessToken="synthetic-access-2")))
        val bodies=mutableListOf<JSONObject>();val keys=mutableListOf<String>()
        coEvery {proof.post("/v1/quality/reports/commands",capture(keys),capture(bodies),local)} answers {receipt()}
        val api=QualityReportHttpClient.signed(proof,auth)
        val signedOwner=api.currentOwner()!!
        api.execute(signedOwner,command());api.execute(signedOwner,command())
        assertEquals(listOf(id,id),keys)
        assertEquals(listOf("synthetic-access-1","synthetic-access-2"),bodies.map {it.getString("access_token")})
        val canonical=bodies.map {JSONObject(it.toString()).apply {remove("access_token");remove("family_id")}.toString()}
        assertEquals(canonical[0],canonical[1])
        verify(exactly=2){auth.refreshSession()}
    }
    @Test fun `logged out or replacement session refuses signed request`()=runTest {
        val proof=mockk<PhotoProofTransport>();val auth=mockk<AuthFlow>()
        every {proof.origin} returns "https://teste.abastevo.com.br"
        every {proof.ownerScope()} returns "account:synthetic:device"
        every {auth.rehydrate()} returns AuthFlow.AuthState.Active(PortableAuth.Session(id,"synthetic","synthetic-access","synthetic-refresh",Instant.now().epochSecond+3600,Instant.now().epochSecond+7200))
        val api=QualityReportHttpClient.signed(proof,auth)
        val signedOwner=api.currentOwner()!!
        every {auth.refreshSession()} returns AuthApiResult.Ok(null)
        assertThrows(QualityReportException::class.java){runBlocking {api.get(signedOwner,id)}}
        every {auth.refreshSession()} returns AuthApiResult.Ok(PortableAuth.Session(id,"other","synthetic-access","synthetic-refresh",Instant.now().epochSecond+3600,Instant.now().epochSecond+7200))
        assertThrows(QualityReportException::class.java){runBlocking {api.get(signedOwner,id)}}
        coVerify(exactly=0){proof.post(any(),any(),any(),any())}
    }
    @Test fun `refresh that replaces the login family cannot dispatch the previous familys intent`()=runTest {
        val proof=mockk<PhotoProofTransport>();val auth=mockk<AuthFlow>()
        var session=PortableAuth.Session(id,"synthetic","synthetic-access","synthetic-refresh",Instant.now().epochSecond+3600,Instant.now().epochSecond+7200)
        every {proof.origin} returns "https://teste.abastevo.com.br"
        every {proof.ownerScope()} returns "account:synthetic:device"
        every {auth.rehydrate()} answers {AuthFlow.AuthState.Active(session)}
        every {auth.refreshSession()} answers {session=session.copy(familyId=station);AuthApiResult.Ok(session)}
        val api=QualityReportHttpClient.signed(proof,auth);val previous=api.currentOwner()!!
        val e=assertThrows(QualityReportException::class.java){runBlocking {api.execute(previous,command())}}
        assertEquals(QualityReportFailure.OWNER_CHANGED,e.kind)
        coVerify(exactly=0){proof.post(any(),any(),any(),any())}
    }

    @Test fun `offline refresh keeps retained owner recoverable without pretending there is live authority`()=runTest {
        val proof=mockk<PhotoProofTransport>();val auth=mockk<AuthFlow>()
        val expired=PortableAuth.Session(id,"synthetic","synthetic-expired-access","synthetic-refresh",1,Instant.now().epochSecond+7200)
        every {proof.origin} returns "https://teste.abastevo.com.br"
        every {proof.ownerScope()} returns "account:synthetic:device"
        every {auth.rehydrate()} returns AuthFlow.AuthState.NeedsRefresh(expired)
        every {auth.refreshSession()} returns AuthApiResult.Err("synthetic-offline")
        val api=QualityReportHttpClient.signed(proof,auth);val owner=api.currentOwner()!!
        val error=assertThrows(QualityReportException::class.java){runBlocking {api.get(owner,id)}}
        assertEquals(QualityReportFailure.UNAVAILABLE,error.kind)
        assertEquals(owner,api.currentOwner())
        coVerify(exactly=0){proof.post(any(),any(),any(),any())}
    }

}
