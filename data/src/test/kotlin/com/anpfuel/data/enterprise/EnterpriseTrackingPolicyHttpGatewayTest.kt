package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.enterprise.tracking.EnterpriseTrackingPolicyHttpGateway
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.tracking.TrackingPolicy
import com.anpfuel.domain.portable.PortableAuth
import io.mockk.every
import io.mockk.mockk
import java.time.Instant
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

/** EMP-GPS-002 (B-BR-EMP-495, EDR-321): the driver's policy read on the account cookie plane, parsed strictly. */
class EnterpriseTrackingPolicyHttpGatewayTest {
    private val a = "7b0e2a54-0000-4000-8000-000000000001"
    private val org = "7b0e2a54-0000-4000-8000-000000000002"
    private val member = "7b0e2a54-0000-4000-8000-000000000003"
    private val origin = "https://api.example.invalid"
    private val readAt = Instant.parse("2026-11-15T12:30:00Z")
    private val cookie = "__Host-empresas-session=abv_company_${"c".repeat(64)}; Path=/; Expires=Thu, 01 Jan 2099 08:00:00 GMT; Secure; HttpOnly; SameSite=Strict"
    private val context = EnterpriseContextSummary(EnterprisePartition.of(a, origin, org), member, "Synthetic company")
    private fun login() = JSONObject().put("account_id", a).put("csrf_token", "abv_company_csrf_${"d".repeat(64)}")
        .put("authenticated_at", "2098-12-31T23:55:00Z").put("expires_at", "2099-01-01T08:00:00Z")
    private fun expected() = JSONObject().put("revision", 2).put("policy", "EXPECTED").put("notice_version", "aviso-2026.1")
        .put("notice_in_force_at", "2026-11-01T00:00:00Z").put("data_mode", "SYNTHETIC")
    private fun off() = JSONObject().put("revision", 0).put("policy", "OFF").put("notice_version", JSONObject.NULL)
        .put("notice_in_force_at", JSONObject.NULL).put("data_mode", "SYNTHETIC")

    private inner class Fixture(private val handle: (Request, Int) -> Response) {
        val fence = EnterpriseSessionFence()
        var stored: PortableAuth.Session? = PortableAuth.Session(a, a, "synthetic-access", "synthetic-refresh", 2000, 4000)
        val sessions = object : AuthSessionStore {
            override fun load() = stored
            override fun save(session: PortableAuth.Session) { stored = session }
            override fun clear() { stored = null }
        }
        val auth = mockk<AuthFlow>().also { every { it.refreshSession() } answers { AuthApiResult.Ok(stored) } }
        val requests = java.util.Collections.synchronizedList(mutableListOf<Request>())
        val client = CompanyAccessClient(OkHttpClient.Builder().addInterceptor(Interceptor { chain ->
            val index = synchronized(requests) { requests.add(chain.request()); requests.size - 1 }
            handle(chain.request(), index)
        }).build(), origin, sessions, auth, fence)
        val gateway = EnterpriseTrackingPolicyHttpGateway(client)
    }
    private fun response(request: Request, body: String, status: Int = 200, cookie: Boolean = false): Response =
        Response.Builder().request(request).protocol(Protocol.HTTP_1_1).code(status).message("Synthetic response")
            .header("Cache-Control", "no-store").body(body.toResponseBody("application/json".toMediaType()))
            .apply { if (cookie) addHeader("Set-Cookie", this@EnterpriseTrackingPolicyHttpGatewayTest.cookie) }.build()
    private fun answering(value: JSONObject, status: Int = 200) = Fixture { request, index ->
        if (index == 0) response(request, login().toString(), cookie = true) else response(request, value.toString(), status)
    }

    @Test fun `the policy in force is read with an empty body and kept with the read instant`() = runBlocking {
        val fixture = answering(expected())
        val read = (fixture.gateway.mine(context, readAt) as EnterpriseAccessResult.Ok).value
        assertEquals(listOf("/v1/org-access/session/login", "/v1/orgs/$org/tracking/policy/mine"), fixture.requests.map { it.url.encodedPath })
        val request = fixture.requests[1]
        assertEquals("{}", Buffer().also { request.body!!.writeTo(it) }.readUtf8())
        assertEquals(cookie.substringBefore(';'), request.header("Cookie"))
        assertEquals(TrackingPolicy.EXPECTED, read.policy); assertEquals(2L, read.revision)
        assertEquals("aviso-2026.1", read.noticeVersion); assertEquals(Instant.parse("2026-11-01T00:00:00Z"), read.noticeInForceAt)
        assertEquals(readAt, read.readAt)
        val none = (answering(off()).gateway.mine(context, readAt) as EnterpriseAccessResult.Ok).value
        assertEquals(TrackingPolicy.OFF, none.policy); assertNull(none.noticeVersion); assertNull(none.noticeInForceAt)
        assertEquals("EnterpriseTrackingPolicyRead(redacted)", read.toString())
    }

    @Test fun `a malformed, inconsistent or refused answer is never a policy`() = runBlocking {
        val malformed = listOf<JSONObject.() -> Unit>(
            { put("extra", 1) }, { remove("data_mode") }, { put("data_mode", "REAL") }, { put("revision", "2") }, { put("revision", 2.5) },
            { put("policy", "ON") }, { put("policy", "expected") }, { put("notice_version", JSONObject.NULL) },
            { put("notice_in_force_at", "2026-11-01 00:00:00") }, { put("notice_version", "Aviso X") }, { put("revision", 0) },
        )
        for ((index, edit) in malformed.withIndex()) {
            assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE),
                answering(expected().apply(edit)).gateway.mine(context, readAt), "case $index")
        }
        assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE),
            answering(off().put("notice_version", "aviso").put("notice_in_force_at", "2026-11-01T00:00:00Z")).gateway.mine(context, readAt))
        val forbidden = answering(JSONObject().put("error", JSONObject().put("code", "FORBIDDEN").put("permission", "trip.trip.start")), 403)
        assertTrue(forbidden.gateway.mine(context, readAt) is EnterpriseAccessResult.Err)
        val other = EnterpriseContextSummary(EnterprisePartition.of(org, origin, org), member, "Synthetic company")
        val unused = answering(expected())
        assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.ACCESS_DENIED), unused.gateway.mine(other, readAt))
    }
}
