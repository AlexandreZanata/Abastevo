package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.portable.PortableAuth
import io.mockk.every
import io.mockk.mockk
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import okhttp3.Call
import okhttp3.Interceptor
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Request
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import okio.Buffer
import org.json.JSONArray
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseContextHttpGatewayTest {
    private val a = "7b0e2a54-0000-4000-8000-000000000001"
    private val b = "7b0e2a54-0000-4000-8000-000000000002"
    private val c = "7b0e2a54-0000-4000-8000-000000000003"
    private val origin = "https://api.example.invalid"
    private val cookie = "__Host-empresas-session=abv_company_${"c".repeat(64)}; Path=/; Expires=Thu, 01 Jan 2099 08:00:00 GMT; Secure; HttpOnly; SameSite=Strict"
    private fun metadata(org: String = a, member: String = b) = JSONObject().put("id", org).put("name", "Synthetic company").put("membership_id", member)
    private fun summary(org: String = a, member: String = b, account: String = a, server: String = origin) =
        EnterpriseContextSummary(EnterprisePartition.of(account, server, org), member, "Synthetic company")
    private fun login() = JSONObject().put("account_id", a).put("csrf_token", "abv_company_csrf_${"d".repeat(64)}")
        .put("authenticated_at", "2098-12-31T23:55:00Z").put("expires_at", "2099-01-01T08:00:00Z")
    private fun page(values: List<JSONObject> = listOf(metadata()), next: String = "") = JSONObject().put("account_id", a)
        .put("organizations", JSONArray(values)).put("next_after_org_id", next)
    private fun read(value: JSONObject = metadata()) = JSONObject().put("account_id", a).put("organization", value)
        .put("permissions", JSONArray().put("idn.employee.read")).put("invitation_grants", JSONArray()).put("invitation_validity_days", 7)
    private fun preview() = JSONObject().put("organization", JSONObject().put("name", "Synthetic company").put("legal_entity_verified", false))
        .put("grants", JSONArray().put(JSONObject().put("role", "DRIVER").put("scope", "SELF")))
        .put("confirmation_required", true).put("consent_digest", "a".repeat(64))
        .put("notice", JSONObject().put("version", EnterpriseInvitationContract.NOTICE_VERSION).put("text", EnterpriseInvitationContract.NOTICE_TEXT))
        .put("data_mode", "SYNTHETIC")

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
        val calls = java.util.Collections.synchronizedList(mutableListOf<Call>())
        val client = CompanyAccessClient(OkHttpClient.Builder().addInterceptor(Interceptor { chain ->
            calls += chain.call()
            val index = synchronized(requests) { requests.add(chain.request()); requests.size - 1 }
            handle(chain.request(), index)
        }).build(), origin, sessions, auth, fence)
        val gateway = EnterpriseContextHttpGateway(client)
        suspend fun open() = (gateway.open() as EnterpriseAccessResult.Ok).value
    }

    private fun response(request: Request, body: String, status: Int = 200, cookie: Boolean = false, json: Boolean = true): Response =
        Response.Builder().request(request).protocol(Protocol.HTTP_1_1).code(status).message("Synthetic response")
            .header("Cache-Control", "no-store").body(body.toResponseBody((if (json) "application/json" else "text/html").toMediaType()))
            .apply { if (cookie) addHeader("Set-Cookie", this@EnterpriseContextHttpGatewayTest.cookie) }.build()
    private fun body(request: Request): JSONObject = JSONObject(Buffer().also { request.body!!.writeTo(it) }.readUtf8())
    private fun error(value: EnterpriseAccessError, result: EnterpriseAccessResult<*>) = assertEquals(EnterpriseAccessResult.Err(value), result)

    @Test fun `actual context DTOs bind account partition membership and expose minimum metadata only`() = runBlocking {
        val fixture = Fixture { request, index -> when (index) {
            0 -> response(request, login().toString(), cookie = true)
            1 -> response(request, page().toString())
            else -> response(request, read().toString())
        } }
        val binding = fixture.open()
        val page = (fixture.gateway.list(binding) as EnterpriseAccessResult.Ok).value
        assertEquals(listOf(summary()), page.contexts)
        assertNull(page.nextAfterOrganizationId)
        val result = fixture.gateway.read(binding, page.contexts.single()) as EnterpriseAccessResult.Ok
        assertEquals(summary(), result.value)
        assertEquals(setOf("limit"), body(fixture.requests[1]).keySet())
        assertEquals(100, body(fixture.requests[1]).getInt("limit"))
        assertEquals(setOf("organization_id", "membership_id"), body(fixture.requests[2]).keySet())
        for (request in fixture.requests.drop(1)) {
            assertEquals(origin, request.header("Origin"))
            assertEquals(cookie.substringBefore(';'), request.header("Cookie"))
            assertEquals("abv_company_csrf_${"d".repeat(64)}", request.header("X-CSRF"))
            assertFalse(body(request).has("account_id"))
            assertNull(request.header("Authorization"))
        }
        assertFalse(result.toString().contains(a))
        assertFalse(result.value.toString().contains("Synthetic"))
    }

    @Test fun `empty scanned pages retain the forward cursor and the next request sends it exactly`() = runBlocking {
        val fixture = Fixture { request, index -> when (index) {
            0 -> response(request, login().toString(), cookie = true)
            1 -> response(request, page(emptyList(), a).toString())
            else -> response(request, page(listOf(metadata(b))).toString())
        } }
        val binding = fixture.open()
        val first = (fixture.gateway.list(binding) as EnterpriseAccessResult.Ok).value
        assertTrue(first.contexts.isEmpty())
        assertEquals(a, first.nextAfterOrganizationId)
        assertEquals(listOf(summary(b)), (fixture.gateway.list(binding, first.nextAfterOrganizationId) as EnterpriseAccessResult.Ok).value.contexts)
        assertEquals(a, body(fixture.requests.last()).getString("after_org_id"))
    }

    @Test fun `generic 403 and 404 context refusal is definitive even without a JSON private body`() = runBlocking {
        for (status in listOf(403, 404)) {
            val fixture = Fixture { request, index -> if (index == 0) response(request, login().toString(), cookie = true)
                else response(request, "Synthetic denial", status, json = false) }
            error(EnterpriseAccessError.ACCESS_DENIED, fixture.gateway.list(fixture.open()))
        }
        val fixture = Fixture { request, index -> if (index == 0) response(request, login().toString(), cookie = true)
            else response(request, "{\"error\":{\"code\":\"UNAVAILABLE\"}}", 503) }
        error(EnterpriseAccessError.UNAVAILABLE, fixture.gateway.read(fixture.open(), summary()))
    }

    @Test fun `wrong account org membership and foreign local selectors refuse without granting a selection`() = runBlocking {
        for (value in listOf(read().put("account_id", b), read(metadata(b)), read(metadata(member = c)))) {
            val fixture = Fixture { request, index -> if (index == 0) response(request, login().toString(), cookie = true) else response(request, value.toString()) }
            error(EnterpriseAccessError.ACCESS_DENIED, fixture.gateway.read(fixture.open(), summary()))
        }
        val fixture = Fixture { request, _ -> response(request, login().toString(), cookie = true) }
        val binding = fixture.open()
        error(EnterpriseAccessError.ACCESS_DENIED, fixture.gateway.read(binding, summary(account = b)))
        error(EnterpriseAccessError.ACCESS_DENIED, fixture.gateway.read(binding, summary(server = "https://other.example.invalid")))
        assertEquals(1, fixture.requests.size)
    }

    @Test fun `duplicate malformed unsorted oversized and backward pages refuse honestly`() = runBlocking {
        val bodies = listOf(
            page(listOf(metadata(), metadata())), page(listOf(metadata(b), metadata())), page(List(101) { metadata() }),
            page().put("next_after_org_id", "invalid"), page().put("next_after_org_id", JSONObject.NULL),
            page(listOf(metadata(b)), a), page(listOf(metadata().put("name", "\nprivate"))),
            page(listOf(metadata().put("membership_id", "bad"))),
        )
        for (value in bodies) {
            val fixture = Fixture { request, index -> if (index == 0) response(request, login().toString(), cookie = true) else response(request, value.toString()) }
            error(EnterpriseAccessError.UNAVAILABLE, fixture.gateway.list(fixture.open()))
        }
        val fixture = Fixture { request, index -> if (index == 0) response(request, login().toString(), cookie = true) else response(request, page(emptyList(), a).toString()) }
        error(EnterpriseAccessError.UNAVAILABLE, fixture.gateway.list(fixture.open(), a))
    }

    @Test fun `background or source identity changes withhold late context reads`() = runBlocking {
        for (mode in listOf("background", "family", "account", "logout")) {
            val entered = CountDownLatch(1)
            val release = CountDownLatch(1)
            val fixture = Fixture { request, index -> if (index == 0) response(request, login().toString(), cookie = true) else {
                entered.countDown(); check(release.await(5, TimeUnit.SECONDS)); response(request, read().toString())
            } }
            val binding = fixture.open()
            val pending = async(Dispatchers.Default) { fixture.gateway.read(binding, summary()) }
            assertTrue(entered.await(5, TimeUnit.SECONDS))
            when (mode) {
                "background" -> fixture.fence.markBackground()
                "family" -> fixture.stored = fixture.stored!!.copy(familyId = b)
                "account" -> fixture.stored = fixture.stored!!.copy(accountId = b)
                else -> fixture.sessions.clear()
            }
            release.countDown()
            error(EnterpriseAccessError.STALE, pending.await())
        }
    }

    @Test fun `outgoing gateway cleanup cannot cancel incoming open or borrow its channel in either direction`() = runBlocking {
        for (direction in listOf("invitation-context", "context-invitation", "context-context")) {
            val entered = CountDownLatch(1)
            val release = CountDownLatch(1)
            val fixture = Fixture { request, index -> when {
                index == 0 -> response(request, login().toString(), cookie = true)
                index == 1 -> { entered.countDown(); check(release.await(5, TimeUnit.SECONDS)); response(request, login().toString(), cookie = true) }
                request.url.encodedPath.endsWith("/invitations/preview") -> response(request, preview().toString())
                else -> response(request, page().toString())
            } }
            val invitation = EnterpriseInvitationHttpGateway(fixture.client)
            val outgoingContext = fixture.gateway
            val incomingContext = EnterpriseContextHttpGateway(fixture.client)
            val first = if (direction == "invitation-context") invitation.open() else outgoingContext.open()
            val oldBinding = (first as EnterpriseAccessResult.Ok).value
            val incoming = async(Dispatchers.Default) { if (direction == "context-invitation") invitation.open() else incomingContext.open() }
            assertTrue(entered.await(5, TimeUnit.SECONDS))
            if (direction == "invitation-context") invitation.invalidate() else outgoingContext.invalidate()
            assertFalse(fixture.calls.last().isCanceled())
            val oldPost = if (direction == "invitation-context") invitation.preview(oldBinding, "code") else outgoingContext.list(oldBinding)
            error(EnterpriseAccessError.STALE, oldPost)
            assertEquals(2, fixture.requests.size)
            release.countDown()
            val current = (incoming.await() as EnterpriseAccessResult.Ok).value
            if (direction == "context-invitation") assertTrue(invitation.preview(current, "code") is EnterpriseAccessResult.Ok)
            else assertTrue(incomingContext.list(current) is EnterpriseAccessResult.Ok)
            assertEquals(3, fixture.requests.size)
        }
    }
}
