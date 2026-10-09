package com.anpfuel.data.remote

import com.anpfuel.application.portable.PhotoCache
import com.anpfuel.data.local.dao.PhotoUploadSessionDao
import com.anpfuel.data.local.entity.PhotoUploadSessionEntity
import com.anpfuel.domain.exception.*
import com.anpfuel.domain.repository.*
import io.mockk.*
import java.io.IOException
import java.time.Instant
import kotlinx.coroutines.test.runTest
import okhttp3.*
import okhttp3.ResponseBody.Companion.toResponseBody
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class ContributionUploadHttpClientTest {
    private val now = 1_791_374_400_123L
    private val capture = "c0000000-0000-4000-8000-000000000001"
    private val upload = "e0000000-0000-4000-8000-000000000001"
    private val evidence = "e0000000-0000-4000-8000-000000000002"
    private val observation = "e0000000-0000-4000-8000-000000000003"
    private val station = "d6c74c23-63db-4c24-a2e5-408cb23bad26"
    private val owner = "synthetic-owner"
    private val origin = "https://api.example.invalid"
    private val proof = mockk<PhotoProofTransport>()
    private val cache = mockk<PhotoCache>()
    private val sessions = mockk<PhotoUploadSessionDao>()
    private var saved: PhotoUploadSessionEntity? = null
    private val puts = mutableListOf<Request>()
    private var putCode = 200
    private val client = OkHttpClient.Builder().addInterceptor { chain ->
        puts += chain.request()
        Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(putCode)
            .message("synthetic").body(ByteArray(0).toResponseBody()).build()
    }.build()
    init {
        every { proof.origin } returns origin
        coEvery { proof.localScope() } returns owner
        every { cache.get("photo-1") } returns ByteArray(16) { 7 }
        coEvery { sessions.find(capture) } answers { saved }
        coEvery { sessions.remember(any()) } coAnswers {
            val row = firstArg<PhotoUploadSessionEntity>()
            if (saved != null && saved!!.copy(evidenceId = null) != row.copy(evidenceId = null)) throw DomainException("changed")
            if (saved == null) saved = row
            saved!!
        }
        coEvery { sessions.ready(capture, upload, evidence) } answers { saved = saved!!.copy(evidenceId=evidence); 1 }
        coEvery { proof.post("/v1/observations", any(), any(), owner) } returns JSONObject().put("id",observation).put("validation_state","RECEIVED")
        coEvery { proof.post("/v1/uploads", "photo:$capture", any(), owner) } returns reservation()
        coEvery { proof.post("/v1/uploads/$upload/complete", "complete:$capture", any(), owner) } returns status("READY")
    }
    private fun gateway() = ContributionUploadHttpClient(proof, cache, sessions, client) { now }
    private fun reservation() = JSONObject().put("upload_id",upload).put("method","PUT")
        .put("url","https://storage.example.invalid/private/object?signature=synthetic")
        .put("required_headers",JSONObject().put("content-type","image/jpeg"))
        .put("expires_at",Instant.ofEpochMilli(now+120000).toString()).put("max_bytes",3145728)
        .put("session_expires_at",Instant.ofEpochMilli(now+86399000).toString())
    private fun status(state: String) = JSONObject().put("upload_id",upload).put("state",state).put("evidence_id",evidence)
    private fun payload(id: String = "row", photo: Boolean = true) = JSONObject()
        .put("client_submission_id",id).put("station_id",station).put("fuel_product","GASOLINE_PREMIUM")
        .put("amount_milli_brl",5890).put("currency","BRL").put("unit","L").put("condition_kind","STANDARD")
        .put("captured_at_millis",now-1000).put("photo_id",if(photo) "photo-1" else JSONObject.NULL)
        .put("photo_capture",if(photo) JSONObject().put("capture_id",capture).put("expires_at_millis",now+86399000) else JSONObject.NULL)
        .put("scope",JSONObject().put("owner_scope",owner).put("origin",origin)).toString()

    @Test fun `configured upload origin must match signed proof before any network request`() {
        val error = assertThrows(IllegalArgumentException::class.java) {
            ContributionUploadHttpClient(proof, cache, sessions, client, origin = "https://other.example.invalid")
        }
        assertEquals("contribution.origin-mismatch", error.message)
        coVerify(exactly = 0) { proof.post(any(), any(), any(), any()) }
        assertTrue(puts.isEmpty())
    }

    @Test fun `unscoped legacy payload never leaves device`() = runTest {
        val doc = JSONObject(payload()).also { it.remove("scope") }
        assertThrows(DomainException::class.java) { kotlinx.coroutines.runBlocking { gateway().submit("row",1,doc.toString(),"attempt") } }
        coVerify(exactly=0) { proof.post(any(),any(),any(),any()) }
        assertTrue(puts.isEmpty())
    }
    @Test fun `two rows share one upload and exact wire receipt survives retry`() = runTest {
        val gateway = gateway()
        val first = gateway.submit("row",1,payload(),"a")
        gateway.submit("other",1,payload("other"),"b")
        gateway.submit("row",1,payload(),"c")
        assertEquals(ContributionRemoteStatus.RECEIVED,first.status)
        assertEquals(observation,first.observationId)
        assertEquals(1,puts.size)
        assertEquals("PUT",puts.single().method)
        assertNull(puts.single().header("Signature")); assertNull(puts.single().header("Authorization"))
        assertEquals("image/jpeg",puts.single().body!!.contentType().toString())
        val wire=mutableListOf<JSONObject>()
        coVerify(exactly=2) { proof.post("/v1/observations","row",capture(wire),owner) }
        assertEquals("GASOLINE_ADDITIVED",wire.last().getString("fuel_product"))
        assertEquals(5890,wire.last().getJSONObject("price").getInt("amount_milli_brl"))
        assertEquals(Instant.ofEpochMilli(now-1000).toString(),wire.last().getString("claimed_captured_at"))
        assertEquals(capture,wire.last().getString("photo_capture_id"))
        assertFalse(wire.last().has("scope")); assertFalse(wire.last().has("photo_id"))
        coVerify(exactly=1) { proof.post("/v1/uploads",any(),any(),owner) }
    }
    @Test fun `missing photo and changed owner or origin never become metadata observations`() = runTest {
        every { cache.get(any()) } returns null
        assertThrows(ContributionPhotoExpired::class.java) { kotlinx.coroutines.runBlocking { gateway().submit("row",1,payload(),"a") } }
        coEvery { proof.localScope() } returns "different-owner"
        assertThrows(DomainException::class.java) { kotlinx.coroutines.runBlocking { gateway().submit("row",1,payload(),"b") } }
        coVerify(exactly=0) { proof.post(any(),any(),any(),any()) }
        assertTrue(puts.isEmpty())
    }
    @Test fun `verifying completion retries through saved status without uploading again`() = runTest {
        coEvery { proof.post("/v1/uploads/$upload/complete",any(),any(),owner) } returns status("VERIFYING")
        assertThrows(IOException::class.java) { kotlinx.coroutines.runBlocking { gateway().submit("row",1,payload(),"a") } }
        coEvery { proof.get("/v1/uploads/$upload",owner) } returns status("READY")
        assertEquals(ContributionRemoteStatus.RECEIVED,gateway().submit("row",1,payload(),"b").status)
        assertEquals(1,puts.size)
    }
    @Test fun `expired receipt refuses upload and put failure never posts observation`() = runTest {
        val doc = JSONObject(payload()).getJSONObject("photo_capture").put("expires_at_millis",now-1)
        val expired = JSONObject(payload()).put("photo_capture",doc)
        assertThrows(ContributionPhotoExpired::class.java) { kotlinx.coroutines.runBlocking { gateway().submit("row",1,expired.toString(),"a") } }
        putCode=500
        assertThrows(IOException::class.java) { kotlinx.coroutines.runBlocking { gateway().submit("row",1,payload(),"a") } }
        coVerify(exactly=0) { proof.post("/v1/observations",any(),any(),any()) }
        assertEquals(1,puts.size)
    }
    @Test fun `server canonical header casing and small clock skew pass upload validation`() = runTest {
        // Exactly what staging mints: "Content-Type" and a 5-minute TTL
        // observed from a phone seconds behind the server.
        coEvery { proof.post("/v1/uploads", any(), any(), owner) } returns reservation()
            .put("required_headers", JSONObject().put("Content-Type", "image/jpeg"))
            .put("expires_at", Instant.ofEpochMilli(now + 302000).toString())
        val receipt = kotlinx.coroutines.runBlocking { gateway().submit("row", 1, payload(), "a") }
        assertEquals(ContributionRemoteStatus.RECEIVED, receipt.status)
        assertEquals(1, puts.size)
    }
    @Test fun `wrong header and overlong authorization stay refused`() = runTest {
        coEvery { proof.get("/v1/uploads/$upload", owner) } returns status("ISSUED")
        coEvery { proof.post("/v1/uploads", any(), any(), owner) } returns reservation()
            .put("required_headers", JSONObject().put("Content-Type", "text/plain"))
        val wrong = assertThrows(IOException::class.java) { kotlinx.coroutines.runBlocking { gateway().submit("row", 1, payload(), "a") } }
        assertEquals("contribution.invalid-upload-authorization", wrong.message)
        coEvery { proof.post("/v1/uploads", any(), any(), owner) } returns reservation()
            .put("expires_at", Instant.ofEpochMilli(now + 400000).toString())
        val overlong = assertThrows(IOException::class.java) { kotlinx.coroutines.runBlocking { gateway().submit("row", 1, payload(), "b") } }
        assertEquals("contribution.invalid-upload-authorization", overlong.message)
        assertTrue(puts.isEmpty())
    }
    @Test fun `fractional money malicious upload and fabricated validation are refused`() = runTest {
        val fractional=JSONObject(payload(photo=false)).put("client_submission_id","row").put("amount_milli_brl",5890.5)
        assertThrows(DomainException::class.java) { kotlinx.coroutines.runBlocking { gateway().submit("row",1,fractional.toString(),"a") } }
        coEvery { proof.post("/v1/uploads",any(),any(),owner) } returns reservation().put("url","http://storage.example.invalid/object?q=x")
        assertThrows(IOException::class.java) { kotlinx.coroutines.runBlocking { gateway().submit("row",1,payload(),"a") } }
        assertTrue(puts.isEmpty())
        coEvery { proof.post("/v1/observations",any(),any(),owner) } returns JSONObject().put("id",observation).put("validation_state","VALIDATED")
        assertThrows(IOException::class.java) { kotlinx.coroutines.runBlocking { gateway().submit("row",1,payload(photo=false),"a") } }
    }
    @Test fun `owner status exact schema validates without media and mismatched fact is refused`() = runTest {
        val receipt=ContributionReceipt("row",1,ContributionRemoteStatus.RECEIVED,observation)
        val fact=JSONObject().put("id",observation).put("station_id",station).put("fuel_product","GASOLINE_ADDITIVED")
            .put("amount_milli_brl",5890).put("unit","L").put("condition",JSONObject().put("kind","STANDARD")).put("validation_state","VALIDATED")
        coEvery { proof.get("/v1/observations/$observation",owner) } returns JSONObject().put("observation",fact).put("validation_state","VALIDATED")
        assertEquals(ContributionRemoteStatus.VALIDATED,gateway().refresh(receipt,payload()).status)
        fact.put("amount_milli_brl",5880)
        assertThrows(IOException::class.java) { kotlinx.coroutines.runBlocking { gateway().refresh(receipt,payload()) } }
        verify(exactly=0) { cache.get(any()) }
        assertTrue(puts.isEmpty())
    }
    @Test fun `true premium grade keeps exact money and distinct signed wire product`() = runTest {
        val payload=JSONObject(payload("premium")).put("fuel_product","GASOLINE_PREMIUM_GRADE").put("amount_milli_brl",9190).toString()
        val receipt=gateway().submit("premium",1,payload,"attempt")
        assertEquals(ContributionRemoteStatus.RECEIVED,receipt.status)
        val bodies=mutableListOf<JSONObject>()
        coVerify(exactly=1) { proof.post("/v1/observations","premium",capture(bodies),owner) }
        assertEquals("GASOLINE_PREMIUM_GRADE",bodies.single().getString("fuel_product"))
        assertEquals(9190,bodies.single().getJSONObject("price").getInt("amount_milli_brl"))
        assertEquals("L",bodies.single().getJSONObject("price").getString("unit"))
        assertEquals(1,puts.size)
    }

}
