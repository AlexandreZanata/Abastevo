package com.anpfuel.data.remote

import com.anpfuel.application.port.CaptureFix
import io.mockk.*
import java.time.Instant
import kotlinx.coroutines.test.runTest
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class PhotoCaptureHttpClientTest {
    private val station = "d6c74c23-63db-4c24-a2e5-408cb23bad26"
    private val transport = mockk<PhotoProofTransport>()
    private val now = System.currentTimeMillis()
    private fun receipt() = JSONObject().put("capture_id", "589a13ba-1f78-41dc-a4ab-d63a511c73da")
        .put("station_id", station).put("issued_at", Instant.ofEpochMilli(now - 1000).toString())
        .put("camera_expires_at", Instant.ofEpochMilli(now + 119000).toString())
        .put("expires_at", Instant.ofEpochMilli(now + 86399000).toString()).put("policy_version", "photo-capture-v1")
    private fun prepare(response: JSONObject) {
        every { transport.origin } returns "https://example.invalid"
        every { transport.ownerScope() } returns "anonymous:synthetic"
        coEvery { transport.identityScope() } returns "anonymous:synthetic"
        coEvery { transport.post(any(), any(), any()) } returns response
    }
    @Test fun `strict signed receipt and transient fix are bound to current owner station origin`() = runTest {
        prepare(receipt())
        val body = slot<JSONObject>()
        coEvery { transport.post("/v1/photo-captures", "capture-client", capture(body)) } returns receipt()
        val client = PhotoCaptureHttpClient(transport)
        val permission = client.authorize(station, "capture-client", CaptureFix(-12.5, -55.7, 10.0, now, true, true, false))
        assertEquals(station, permission.stationId)
        assertTrue(client.isCurrent(permission))
        assertEquals(now, Instant.parse(body.captured.getJSONObject("location").getString("captured_at")).toEpochMilli())
        every { transport.ownerScope() } returns "anonymous:other"
        assertFalse(client.isCurrent(permission))
        every { transport.ownerScope() } returns "anonymous:synthetic"
        every { transport.origin } returns "https://other.invalid"
        assertFalse(client.isCurrent(permission))
    }
    @Test fun `cross station stale longer deadlines changed identity and wrong policy are refused`() = runTest {
        listOf(
            receipt().put("station_id", "e7d85d34-74ec-5d35-b3f6-519dc44ce370"),
            receipt().put("camera_expires_at", Instant.ofEpochMilli(now - 1).toString()),
            receipt().put("camera_expires_at", Instant.ofEpochMilli(now + 120001).toString()),
            receipt().put("expires_at", Instant.ofEpochMilli(now + 86400001).toString()),
            receipt().put("policy_version", "different"),
        ).forEach { response ->
            prepare(response)
            try {
                PhotoCaptureHttpClient(transport).authorize(station, "capture-client", CaptureFix(-12.5,-55.7,10.0,now,true,true,false))
                fail<Unit>("invalid receipt accepted")
            } catch (_: java.io.IOException) { }
        }
        prepare(receipt())
        every { transport.ownerScope() } returns "anonymous:changed"
        try {
            PhotoCaptureHttpClient(transport).authorize(station, "capture-client", CaptureFix(-12.5,-55.7,10.0,now,true,true,false))
            fail<Unit>("changed owner accepted")
        } catch (_: java.io.IOException) { }
    }
}
