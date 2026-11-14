package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.enterprise.EnterpriseOfflineLease
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.portable.PortableAuth
import io.mockk.every
import io.mockk.mockk
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import okhttp3.Interceptor
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Request
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import okio.Buffer
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseDeviceContactClientTest {
    private val account = "7b0e2a54-0000-4000-8000-000000000001"
    private val org = "7b0e2a54-0000-4000-8000-000000000002"
    private val member = "7b0e2a54-0000-4000-8000-000000000003"
    private val enrollment = "7b0e2a54-0000-4000-8000-000000000004"
    private val operation = "7b0e2a54-0000-4000-8000-000000000005"
    private val other = "7b0e2a54-0000-4000-8000-000000000006"
    private val origin = "https://api.example.invalid"
    private val target = EnterpriseDeviceTarget(EnterprisePartition.of(account, origin, org), member)
    private fun binding(epoch: Long = 0) = EnterpriseAccessBinding(account, origin, account, epoch)
    private fun request(known: Boolean = true) = EnterpriseContactRequest(binding(), target,
        if (known) EnterpriseEnrollmentBinding(target, enrollment, 1) else null)

    private fun active() = JSONObject().put("schema_version", 1).put("request_id", operation).put("account_id", account)
        .put("api_origin", origin).put("org_id", org).put("membership_id", member).put("enrollment_id", enrollment)
        .put("family_id", account).put("enrollment_revision", "1").put("server_time", "2026-11-13T12:00:00Z")
        .put("verdict", "ACTIVE").put("access_model", "EXISTING_ACCOUNT").put("offline_validity_seconds", 2592000)
        .put("managed_unlock_seconds", JSONObject.NULL)
    private fun terminal(verdict: String) = active().apply {
        remove("access_model"); remove("offline_validity_seconds"); remove("managed_unlock_seconds")
        put("verdict", verdict); put("enrollment_revision", "2")
        put("revocation_class", if (verdict == "ERASE") "LOST_OR_COMPROMISED" else "ACCESS_ENDED")
    }
    private fun device() = JSONObject().put("id", enrollment).put("membership_id", member).put("label", "Android")
        .put("state", "ENROLLED").put("revision", 1).put("revocation_class", "")
        .put("enrolled_at", "2026-11-13T12:00:00Z").put("last_sync_at", JSONObject.NULL)
        .put("revoked_at", JSONObject.NULL).put("data_mode", "SYNTHETIC")

    private fun response(request: Request, body: String, code: Int = 200, headers: Map<String, String> = emptyMap()): Response =
        Response.Builder().request(request).protocol(Protocol.HTTP_1_1).code(code).message("Synthetic response")
            .header("Cache-Control", "no-store").header("Content-Type", "application/json")
            .body(body.toResponseBody("application/json".toMediaType()))
            .apply { headers.forEach { (key, value) -> header(key, value) } }.build()
    private fun body(request: Request) = JSONObject(Buffer().also { request.body!!.writeTo(it) }.readUtf8())

    private inner class Fixture(handle: (Request, Int) -> Response) {
        val fence = EnterpriseSessionFence()
        @Volatile var stored: PortableAuth.Session? = PortableAuth.Session(account, account, "synthetic-access", "synthetic-refresh", 2000, 4000)
        val sessions = object : AuthSessionStore {
            override fun load() = stored
            override fun save(session: PortableAuth.Session) { stored = session }
            override fun clear() { stored = null }
        }
        val auth = mockk<AuthFlow>().also { every { it.refreshSession() } answers { AuthApiResult.Ok(stored) } }
        val requests = java.util.Collections.synchronizedList(mutableListOf<Request>())
        val signed = java.util.Collections.synchronizedList(mutableListOf<ByteArray>())
        val client = EnterpriseDeviceContactClient(OkHttpClient.Builder().addInterceptor(Interceptor { chain ->
            val index = synchronized(requests) { requests.add(chain.request()); requests.size - 1 }
            handle(chain.request(), index)
        }).build(), origin, sessions, auth, fence, EnterpriseDeviceSigner { selected, _, bytes ->
            assertEquals(binding(fence.epoch), selected)
            signed += bytes.copyOf()
            mapOf("Signature" to "synthetic-signature", "Signature-Created" to "1794571200",
                "Signature-Expires" to "1794571320", "Signature-Keyid" to "synthetic-key", "Signature-Nonce" to "synthetic-nonce")
        }, { operation })
    }

    @Test fun `known encrypted enrollment contacts only its exact retained tuple without cookies`() = runBlocking {
        val fixture = Fixture { sent, _ -> response(sent, active().toString()) }
        val result = fixture.client.confirm(request()) as EnterpriseContactReply.Active
        assertEquals(EnterpriseEnrollmentBinding(target, enrollment, 1), result.grant.enrollment)
        assertEquals(EnterpriseOfflineLease.DEFAULT_VALIDITY_MILLIS, result.grant.validityMillis)
        assertNull(result.grant.managedUnlockSeconds)
        assertEquals(1, fixture.requests.size)
        val sent = fixture.requests.single()
        assertEquals("/v1/orgs/$org/devices/contact", sent.url.encodedPath)
        assertEquals(setOf("schema_version", "request_id", "family_id", "access_token", "membership_id", "enrollment_id"), body(sent).keySet())
        assertEquals("synthetic-access", body(sent).getString("access_token"))
        assertEquals(origin, sent.header("Origin"))
        assertNull(sent.header("Cookie")); assertNull(sent.header("Authorization")); assertNull(sent.header("X-CSRF"))
        assertArrayEquals(fixture.signed.single(), Buffer().also { sent.body!!.writeTo(it) }.readByteArray())
        assertFalse(result.toString().contains(account))
    }

    @Test fun `new context checks actual signed enrollment and enrolls only after source absence`() = runBlocking {
        for (alreadyEnrolled in listOf(true, false)) {
            val fixture = Fixture { sent, _ -> when {
                sent.url.encodedPath.endsWith("/check") -> if (alreadyEnrolled) response(sent, device().toString()) else response(sent, "{}", 404)
                sent.url.encodedPath.endsWith("/enroll") -> response(sent, JSONObject().put("device", device()).put("operation_id", operation)
                    .put("reason", "").put("replayed", false).put("data_mode", "SYNTHETIC").toString())
                else -> response(sent, active().toString())
            } }
            assertTrue(fixture.client.confirm(request(known = false)) is EnterpriseContactReply.Active)
            assertEquals(if (alreadyEnrolled) listOf("check", "contact") else listOf("check", "enroll", "contact"), fixture.requests.map { it.url.encodedPath.substringAfterLast('/') })
            fixture.requests.firstOrNull { it.url.encodedPath.endsWith("/enroll") }?.let {
                assertEquals(operation, body(it).getString("operation_id")); assertEquals("Android", body(it).getString("label"))
            }
        }
    }

    @Test fun `only exact matching terminal class is returned and generic errors never bootstrap or erase`() = runBlocking {
        for (verdict in listOf("ERASE", "LOCK")) {
            val fixture = Fixture { sent, _ -> response(sent, terminal(verdict).toString()) }
            val reply = fixture.client.confirm(request())
            assertEquals(verdict == "ERASE", reply is EnterpriseContactReply.Erase)
            assertEquals(verdict == "LOCK", reply is EnterpriseContactReply.Lock)
        }
        for (code in listOf(401, 403, 404, 409, 429, 503)) {
            val fixture = Fixture { sent, _ -> response(sent, "{}", code) }
            assertThrows(EnterpriseProtectionRefused::class.java) { runBlocking { fixture.client.confirm(request()) } }
            assertEquals(1, fixture.requests.size)
            assertTrue(fixture.requests.single().url.encodedPath.endsWith("/contact"))
        }
    }

    @Test fun `all exact response bindings and canonical policy fields are mandatory`() = runBlocking {
        val mutations: List<(JSONObject) -> Unit> = listOf(
            { it.put("request_id", other) }, { it.put("account_id", other) }, { it.put("api_origin", "$origin/") },
            { it.put("org_id", other) }, { it.put("membership_id", other) }, { it.put("enrollment_id", other) },
            { it.put("family_id", other) }, { it.put("enrollment_revision", "0") }, { it.put("enrollment_revision", "01") },
            { it.put("enrollment_revision", "9223372036854775808") }, { it.put("enrollment_revision", 1) },
            { it.put("server_time", "2026-11-13T12:00:00.000Z") }, { it.put("server_time", "2026-11-13T12:00:00+00:00") },
            { it.put("offline_validity_seconds", 900) }, { it.put("managed_unlock_seconds", 60) },
            { it.remove("managed_unlock_seconds") }, { it.put("access_model", "MANAGED") }, { it.put("unknown", true) },
        )
        for (mutate in mutations) {
            val value = active().also(mutate)
            val fixture = Fixture { sent, _ -> response(sent, value.toString()) }
            assertThrows(EnterpriseProtectionRefused::class.java) { runBlocking { fixture.client.confirm(request()) } }
        }
        for (bad in listOf(terminal("ERASE").put("revocation_class", "ACCESS_ENDED"), terminal("LOCK").put("offline_validity_seconds", 2592000))) {
            val fixture = Fixture { sent, _ -> response(sent, bad.toString()) }
            assertThrows(EnterpriseProtectionRefused::class.java) { runBlocking { fixture.client.confirm(request()) } }
        }
    }

    @Test fun `malformed duplicate oversized cacheable cookie and redirect responses are non destructive`() = runBlocking {
        val cases = listOf(
            Triple(active().toString().dropLast(1) + ",\"verdict\":\"ERASE\"}", 200, emptyMap()),
            Triple(" ".repeat(65537), 200, emptyMap()),
            Triple(active().toString(), 200, mapOf("Cache-Control" to "public")),
            Triple(active().toString(), 200, mapOf("Set-Cookie" to "unexpected=synthetic")),
            Triple(active().toString(), 302, mapOf("Location" to "https://other.example.invalid/")),
        )
        for ((value, code, headers) in cases) {
            val fixture = Fixture { sent, _ -> response(sent, value, code, headers) }
            assertThrows(EnterpriseProtectionRefused::class.java) { runBlocking { fixture.client.confirm(request()) } }
            assertEquals(1, fixture.requests.size)
        }
    }

    @Test fun `late terminal response after background account family or token replacement is withheld`() = runBlocking {
        val changes: List<(Fixture) -> Unit> = listOf(
            { it.fence.markBackground() }, { it.fence.lock() }, { it.stored = it.stored!!.copy(accountId = other) },
            { it.stored = it.stored!!.copy(familyId = other) }, { it.stored = it.stored!!.copy(accessToken = "replacement-access") },
        )
        for (change in changes) {
            val entered = CountDownLatch(1); val release = CountDownLatch(1)
            val fixture = Fixture { sent, _ -> entered.countDown(); check(release.await(5, TimeUnit.SECONDS)); response(sent, terminal("ERASE").toString()) }
            val pending = async(Dispatchers.IO) { runCatching { fixture.client.confirm(request()) } }
            assertTrue(entered.await(5, TimeUnit.SECONDS))
            change(fixture); release.countDown()
            val failure = pending.await().exceptionOrNull() as? EnterpriseProtectionRefused
            assertEquals(EnterpriseAccessError.STALE, failure?.error)
        }
    }
}
