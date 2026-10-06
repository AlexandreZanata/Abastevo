package com.anpfuel.data.remote.profile

import com.anpfuel.domain.portable.PortableAuth
import java.util.Base64
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class ProfileClaimHttpClientTest {
    private val id = "d6c74c23-63db-4c24-a2e5-408cb23bad26"
    private val session = PortableAuth.Session("family", "account", "access", "refresh", 2000, 5000)
    private fun response() = MockResponse().setBody("""{"id":"$id","station_id":"$id","role":"manager","scopes":["profile.edit"],"state":"draft","declaration_id":"$id","declaration_state":"active","declaration":"exact","expires_at":"2026-10-05T12:30:00Z"}""")
    @Test fun `open uses real scopes then proof sends original bytes and no-store owner auth`() = runTest {
        MockWebServer().use { server ->
            val api = ProfileClaimHttpClient(OkHttpClient(), server.url("/").toString())
            server.enqueue(response())
            val claim = api.open(session, id, "manager", setOf("profile.edit"), "key")
            val open = server.takeRequest()
            assertEquals("/v1/stations/$id/claims", open.path)
            assertEquals("profile.edit", JSONObject(open.body.readUtf8()).getJSONArray("scopes").getString(0))
            assertTrue(claim.canSupplyProof)
            server.enqueue(MockResponse().setResponseCode(201).setBody("""{"id":"$id","status":"received"}"""))
            val bytes = "%PDF-original".toByteArray()
            api.submit(session, claim, bytes)
            val sent = server.takeRequest()
            assertEquals("/v1/profile/claims/$id/proof", sent.path)
            assertEquals("no-store", sent.getHeader("Cache-Control"))
            val payload = JSONObject(sent.body.readUtf8())
            assertEquals("access", payload.getString("access_token"))
            assertEquals(id, payload.getString("declaration_id"))
            assertEquals("scan", payload.getString("kind"))
            assertArrayEquals(bytes, Base64.getDecoder().decode(payload.getString("content_base64")))
        }
    }
    @Test fun `server refusal never becomes optimistic acknowledgment and details stay redacted`() = runTest {
        MockWebServer().use { server ->
            val api = ProfileClaimHttpClient(OkHttpClient(), server.url("/").toString())
            server.enqueue(MockResponse().setResponseCode(403).setBody("private document"))
            val error = assertThrows(ProfileHttpFailure::class.java) { kotlinx.coroutines.runBlocking { api.cancel(session, id) } }
            assertEquals(403, error.status)
            assertFalse(error.message.orEmpty().contains("private"))
        }
    }
}
