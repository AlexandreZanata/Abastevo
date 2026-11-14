package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.portable.*
import com.anpfuel.data.local.auth.SerializedAuthOperations
import com.anpfuel.domain.portable.PortableAuth
import io.mockk.every
import io.mockk.mockk
import java.io.IOException
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import kotlinx.coroutines.asCoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import okhttp3.Call
import okhttp3.Callback
import okhttp3.Interceptor
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Request
import okhttp3.Response
import okhttp3.ResponseBody
import okhttp3.ResponseBody.Companion.toResponseBody
import okhttp3.MediaType.Companion.toMediaType
import okio.Buffer
import okio.BufferedSource
import okio.Source
import okio.Timeout
import okio.buffer
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseInvitationHttpGatewayTest {
    private val a = "7b0e2a54-0000-4000-8000-000000000001"
    private val b = "7b0e2a54-0000-4000-8000-000000000002"
    private val origin = "https://api.example.invalid"
    private val code = "AIN1.$a.${"e".repeat(64)}"
    private val cookie = "__Host-empresas-session=abv_company_${"c".repeat(64)}; Path=/; Expires=Thu, 01 Jan 2099 08:00:00 GMT; Secure; HttpOnly; SameSite=Strict"
    private fun login() = JSONObject().put("account_id", a).put("csrf_token", "abv_company_csrf_${"d".repeat(64)}")
        .put("authenticated_at", "2099-01-01T00:00:00Z").put("expires_at", "2099-01-01T08:00:00Z")
    private fun preview() = JSONObject().put("organization", JSONObject().put("name", "Synthetic company").put("legal_entity_verified", false))
        .put("grants", org.json.JSONArray().put(JSONObject().put("role", "DRIVER").put("scope", "SELF")))
        .put("confirmation_required", true).put("consent_digest", "a".repeat(64))
        .put("notice", JSONObject().put("version", EnterpriseInvitationContract.NOTICE_VERSION).put("text", EnterpriseInvitationContract.NOTICE_TEXT))
        .put("data_mode", "SYNTHETIC")
    private fun ack(state: String = "ACCEPTED", membership: String = "INVITED") = JSONObject()
        .put("organization_id", a).put("invitation_id", a).put("membership_id", b)
        .put("state", state).put("membership_state", membership).put("replayed", false).put("data_mode", "SYNTHETIC")
        .apply { if (state == "ACCEPTED") put("accepted_at", "2099-01-01T00:00:00Z").put("confirmation_deadline", "2099-01-08T00:00:00Z") }

    private inner class Fixture(private val handle: (Request, Int) -> Response) {
        val fence = EnterpriseSessionFence()
        var stored: PortableAuth.Session? = PortableAuth.Session(a, a, "synthetic-access", "synthetic-refresh", 2000, 4000)
        val sessions: AuthSessionStore = EnterpriseFencedSessionStore(object : AuthSessionStore {
            override fun load() = stored
            override fun save(session: PortableAuth.Session) { stored = session }
            override fun clear() { stored = null }
        }, fence)
        val api = mockk<AuthAccountApi>()
        val requests = java.util.Collections.synchronizedList(mutableListOf<Request>())
        val receivedCalls = java.util.Collections.synchronizedList(mutableListOf<Call>())
        private val key = object : AuthKeyStore {
            override fun loadKey(): AuthFlow.KeyBackup? = null
            override fun saveKey(backup: AuthFlow.KeyBackup) = Unit
            override fun clearKey() = Unit
        }
        val auth = AuthFlow(AuthPorts(sessions, key, AuthWallClock { 100 }, PortableNonceSource { a }, api, SerializedAuthOperations()))
        val client = CompanyAccessClient(OkHttpClient.Builder().addInterceptor(Interceptor { chain ->
            receivedCalls.add(chain.call())
            val index = synchronized(requests) { requests.add(chain.request()); requests.size - 1 }
            handle(chain.request(), index)
        }).build(), origin, sessions, auth, fence)
        val gateway = EnterpriseInvitationHttpGateway(client)
        suspend fun open(): EnterpriseAccessBinding = (gateway.open() as EnterpriseAccessResult.Ok).value
    }

    private fun response(request: Request, body: String, status: Int = 200, setCookie: List<String> = emptyList(), json: Boolean = true, noStore: Boolean = true): Response =
        Response.Builder().request(request).protocol(Protocol.HTTP_1_1).code(status).message("Synthetic response")
            .body(body.toResponseBody((if (json) "application/json" else "text/plain").toMediaType()))
            .apply { if (noStore) header("Cache-Control", "no-store"); setCookie.forEach { addHeader("Set-Cookie", it) } }.build()

    private fun requestBody(request: Request): JSONObject = JSONObject(Buffer().also { request.body!!.writeTo(it) }.readUtf8())
    private fun assertError(expected: EnterpriseAccessError, result: EnterpriseAccessResult<*>) = assertEquals(EnterpriseAccessResult.Err(expected), result)

    @Test fun `real transport sends dedicated cookie and CSRF only after source login and acknowledges waiting`() = runBlocking {
        val fixture = Fixture { request, index -> when (index) {
            0 -> response(request, login().toString(), setCookie = listOf(cookie))
            1 -> response(request, preview().toString())
            else -> response(request, ack().toString())
        } }
        val binding = fixture.open()
        assertEquals(binding, fixture.gateway.currentBinding())
        assertTrue(fixture.gateway.preview(binding, "opaque-code") is EnterpriseAccessResult.Ok)
        val accepted = fixture.gateway.accept(binding, b, code, "a".repeat(64)) as EnterpriseAccessResult.Ok
        assertTrue(accepted.value.awaitingConfirmation)
        fixture.requests.forEach {
            assertEquals(origin, it.header("Origin"))
            assertEquals("no-store", it.header("Cache-Control"))
            assertNull(it.header("Authorization")); assertNull(it.header("Signature"))
            assertNull(it.url.query)
        }
        val login = fixture.requests[0]
        assertEquals(setOf("family_id", "access_token"), requestBody(login).keySet())
        assertNull(login.header("Cookie")); assertNull(login.header("X-CSRF"))
        for (request in fixture.requests.drop(1)) {
            assertEquals(cookie.substringBefore(';'), request.header("Cookie"))
            assertEquals("abv_company_csrf_${"d".repeat(64)}", request.header("X-CSRF"))
            assertFalse(requestBody(request).toString().contains("synthetic-access"))
            assertFalse(requestBody(request).has("account_id"))
        }
        assertEquals(setOf("schema_version", "operation_id", "code", "consent", "consent_digest"), requestBody(fixture.requests[2]).keySet())
        assertFalse(fixture.client.toString().contains("api.example"))
        assertFalse(accepted.toString().contains(a))
    }

    @Test fun `decline has no consent or credentials and never returns active membership`() = runBlocking {
        val fixture = Fixture { request, index -> if (index == 0) response(request, login().toString(), setCookie = listOf(cookie))
            else response(request, ack("DECLINED", "ENDED").toString()) }
        val result = fixture.gateway.decline(fixture.open(), b, code) as EnterpriseAccessResult.Ok
        assertFalse(result.value.active)
        assertEquals(setOf("schema_version", "operation_id", "code"), requestBody(fixture.requests.last()).keySet())
    }

    @Test fun `source cookie lifetime begins at creation after personal authentication`() = runBlocking {
        // Actual companysession.Service.Login uses created(now)+8h. AuthenticatedAt can be older.
        val sourceLogin = login().put("authenticated_at", "2098-12-31T23:55:00Z")
        val fixture = Fixture { request, index -> if (index == 0) response(request, sourceLogin.toString(), setCookie = listOf(cookie))
            else response(request, preview().toString()) }
        val binding = fixture.open()
        assertTrue(fixture.gateway.preview(binding, code) is EnterpriseAccessResult.Ok)
        assertEquals(2, fixture.requests.size)
    }

    @Test fun `missing source proof foreign bindings and invalid origins never send`() = runBlocking {
        val fixture = Fixture { _, _ -> error("Network must not run") }
        fixture.sessions.clear()
        assertError(EnterpriseAccessError.SIGN_IN_REQUIRED, fixture.gateway.open())
        assertError(EnterpriseAccessError.STALE, fixture.gateway.preview(EnterpriseAccessBinding(a, origin, a, 0), "code"))
        assertTrue(fixture.requests.isEmpty())
        for (server in listOf("http://api.example.invalid", "https://api.example.invalid/v1", "https://api.example.invalid?secret=value")) {
            assertThrows(IllegalArgumentException::class.java) { CompanyAccessClient(OkHttpClient(), server, fixture.sessions, fixture.auth, fixture.fence) }
        }
    }

    @Test fun `invalid foreign weak duplicate cookie csrf account and login time refuse closed`() = runBlocking {
        val cookies = listOf(
            emptyList(), listOf(cookie, cookie), listOf(cookie.replace("; Secure", "")), listOf(cookie.replace("; HttpOnly", "")),
            listOf(cookie.replace("SameSite=Strict", "SameSite=Lax")), listOf(cookie.replace("Path=/;", "Path=/v1;")),
            listOf(cookie + "; Domain=foreign.example.invalid"), listOf(cookie + "; Domain=api.example.invalid"),
            listOf(cookie.replace("abv_company_", "unexpected_")),
        )
        for (headers in cookies) {
            val fixture = Fixture { request, _ -> response(request, login().toString(), setCookie = headers) }
            assertError(EnterpriseAccessError.UNAVAILABLE, fixture.gateway.open())
            assertError(EnterpriseAccessError.STALE, fixture.gateway.preview(fixture.gateway.currentBinding()!!, "code"))
        }
        for (body in listOf(login().put("account_id", b), login().put("csrf_token", "private-invalid"), login().put("expires_at", "2099-01-01T00:00:00Z"), login().put("authenticated_at", "invalid"))) {
            val fixture = Fixture { request, _ -> response(request, body.toString(), setCookie = listOf(cookie)) }
            assertError(EnterpriseAccessError.UNAVAILABLE, fixture.gateway.open())
        }
    }

    @Test fun `unknown notices malformed grants booleans and nonsynthetic previews never become consent`() = runBlocking {
        val bodies = listOf(
            preview().put("data_mode", "REAL"), preview().put("consent_digest", "A".repeat(64)), preview().put("confirmation_required", "true"),
            preview().apply { getJSONObject("notice").put("version", "future") },
            preview().apply { getJSONObject("notice").put("text", "Changed collection policy") },
            preview().apply { getJSONObject("organization").put("legal_entity_verified", "false") },
            preview().apply { getJSONArray("grants").getJSONObject(0).put("scope", "STATION") },
            preview().apply { getJSONArray("grants").getJSONObject(0).put("role", "UNKNOWN") },
            preview().apply { getJSONArray("grants").put(getJSONArray("grants").getJSONObject(0)) },
        )
        for (body in bodies) {
            val fixture = Fixture { request, index -> if (index == 0) response(request, login().toString(), setCookie = listOf(cookie)) else response(request, body.toString()) }
            assertError(EnterpriseAccessError.UNAVAILABLE, fixture.gateway.preview(fixture.open(), "code"))
        }
    }

    @Test fun `unbound malformed and wrong decision acknowledgements never report success`() = runBlocking {
        for (body in listOf(ack().put("organization_id", "bad"), ack().put("invitation_id", b), ack().put("state", "DECLINED"), ack().put("membership_state", "ENDED"),
            ack().put("replayed", "false"), ack().put("confirmation_deadline", "2099-01-09T00:00:00Z"), ack().put("data_mode", "REAL"))) {
            val fixture = Fixture { request, index -> if (index == 0) response(request, login().toString(), setCookie = listOf(cookie)) else response(request, body.toString()) }
            assertError(EnterpriseAccessError.UNAVAILABLE, fixture.gateway.accept(fixture.open(), b, code, "a".repeat(64)))
        }
    }

    @Test fun `bounded JSON cache policy redirects replacement cookies and transport failure never leak`() = runBlocking {
        for (mode in listOf("oversize", "nonjson", "nostore", "redirect", "malformed", "cookie", "failure")) {
            val fixture = Fixture { request, index -> if (index == 0) response(request, login().toString(), setCookie = listOf(cookie)) else when (mode) {
                "oversize" -> response(request, "x".repeat(65537))
                "nonjson" -> response(request, preview().toString(), json = false)
                "nostore" -> response(request, preview().toString(), noStore = false)
                "redirect" -> response(request, "{}", status = 302).newBuilder().header("Location", "https://foreign.example.invalid/secret").build()
                "malformed" -> response(request, "PRIVATE-BODY{")
                "cookie" -> response(request, preview().toString(), setCookie = listOf(cookie))
                else -> throw IOException("PRIVATE-CREDENTIAL")
            } }
            assertError(EnterpriseAccessError.UNAVAILABLE, fixture.gateway.preview(fixture.open(), "code"))
            assertEquals(2, fixture.requests.size)
        }
        val fixture = Fixture { request, _ -> response(request, login().toString(), setCookie = listOf(cookie)) }
        val selected = fixture.open()
        assertError(EnterpriseAccessError.UNAVAILABLE, fixture.gateway.preview(selected, "x".repeat(16385)))
        assertEquals(1, fixture.requests.size)
    }

    @Test fun `uniform server refusals retain only registered status bound codes`() = runBlocking {
        for ((status, code, expected) in listOf(
            Triple(404, "TOKEN_INVALID", EnterpriseAccessError.INVALID_CODE), Triple(401, "UNAUTHENTICATED", EnterpriseAccessError.SIGN_IN_REQUIRED),
            Triple(401, "FRESH_AUTH_REQUIRED", EnterpriseAccessError.FRESH_AUTH_REQUIRED), Triple(409, "CONFLICT", EnterpriseAccessError.CONFLICT),
            Triple(409, "ALREADY_MEMBER", EnterpriseAccessError.ALREADY_MEMBER), Triple(403, "MANAGED_ACCOUNT_RESTRICTED", EnterpriseAccessError.MANAGED_RESTRICTED),
            Triple(429, "RATE_LIMITED", EnterpriseAccessError.RATE_LIMITED), Triple(503, "PRIVATE-ERROR", EnterpriseAccessError.UNAVAILABLE),
            Triple(200, "TOKEN_INVALID", EnterpriseAccessError.UNAVAILABLE),
        )) {
            val fixture = Fixture { request, index -> if (index == 0) response(request, login().toString(), setCookie = listOf(cookie))
                else response(request, JSONObject().put("error", JSONObject().put("code", code).put("message", "PRIVATE-ERROR")).toString(), status) }
            val result = fixture.gateway.preview(fixture.open(), "code")
            assertError(expected, result)
            assertFalse(result.toString().contains("PRIVATE"))
        }
    }

    @Test fun `account family background and explicit invalidation withhold late HTTP receipts`() = runBlocking {
        for (mode in listOf("account", "family", "background", "invalidate", "logout")) {
            val entered = CountDownLatch(1)
            val release = CountDownLatch(1)
            val fixture = Fixture { request, index -> if (index == 0) response(request, login().toString(), setCookie = listOf(cookie)) else {
                entered.countDown(); check(release.await(5, TimeUnit.SECONDS)); response(request, ack().toString())
            } }
            val selected = fixture.open()
            val pending = async(Dispatchers.Default) { fixture.gateway.accept(selected, b, code, "a".repeat(64)) }
            assertTrue(entered.await(5, TimeUnit.SECONDS))
            when (mode) {
                "account" -> fixture.sessions.save(fixture.stored!!.copy(accountId = b))
                "family" -> fixture.sessions.save(fixture.stored!!.copy(familyId = b))
                "background" -> fixture.fence.markBackground()
                "invalidate" -> fixture.gateway.invalidate()
                else -> fixture.sessions.clear()
            }
            release.countDown()
            assertError(EnterpriseAccessError.STALE, pending.await())
        }
    }

    @Test fun `family renewal and background refresh refuse old ceremony before login`() = runBlocking {
        for (background in listOf(false, true)) {
            val fixture = Fixture { request, _ -> response(request, login().toString(), setCookie = listOf(cookie)) }
            fixture.stored = fixture.stored!!.copy(accessExpiresAt = 100)
            every { fixture.api.refresh(a, "synthetic-refresh") } answers {
                if (background) fixture.fence.markBackground()
                AuthApiResult.Ok(fixture.stored!!.copy(familyId = b, accessToken = "rotated-access", accessExpiresAt = 2000))
            }
            val result = fixture.gateway.open()
            assertError(EnterpriseAccessError.STALE, result)
            assertTrue(fixture.requests.isEmpty())
            val retried = fixture.gateway.open() as EnterpriseAccessResult.Ok
            assertEquals(b, retried.value.familyId)
            assertEquals(if (background) 2L else 1L, retried.value.epoch)
        }
    }

    @Test fun `more than ten previews restart by rotating only the same account cookie`() = runBlocking {
        var issued = 0
        var prior: String? = null
        val fixture = Fixture { request, _ ->
            if (request.url.encodedPath.endsWith("/session/login")) {
                assertEquals(prior, request.header("Cookie"))
                assertNull(request.header("X-CSRF"))
                issued++
                val current = "__Host-empresas-session=abv_company_${issued.toString(16).padStart(64, '0')}"
                prior = current
                response(request, login().toString(), setCookie = listOf(cookie.replace(cookie.substringBefore(';'), current)))
            } else {
                assertEquals(prior, request.header("Cookie"))
                response(request, preview().toString())
            }
        }
        val flow = EnterpriseInvitationFlow(fixture.gateway) { b }
        repeat(14) {
            flow.preview(code)
            assertEquals(EnterpriseInvitationStage.PREVIEW, flow.state.value.stage)
            flow.restart()
        }
        assertEquals(14, issued)
        assertError(EnterpriseAccessError.STALE, fixture.gateway.preview(fixture.gateway.currentBinding()!!, code))
        assertEquals(28, fixture.requests.size)
    }

    @Test fun `retirement cookie never crosses foreign account logout or private channel invalidation`() = runBlocking {
        lateinit var fixture: Fixture
        fixture = Fixture { request, _ -> response(request, login().put("account_id", fixture.stored!!.accountId).toString(), setCookie = listOf(cookie)) }
        val original = fixture.stored!!
        val binding = fixture.open()
        fixture.gateway.invalidate()
        assertError(EnterpriseAccessError.STALE, fixture.gateway.preview(binding, code))
        fixture.sessions.save(original.copy(familyId = b))
        fixture.open()
        assertEquals(cookie.substringBefore(';'), fixture.requests.last().header("Cookie"))
        fixture.sessions.save(original.copy(accountId = b, familyId = b))
        fixture.gateway.invalidate()
        fixture.open()
        assertNull(fixture.requests.last().header("Cookie"))
        fixture.sessions.clear()
        assertError(EnterpriseAccessError.SIGN_IN_REQUIRED, fixture.gateway.open())
        fixture.sessions.save(original)
        fixture.open()
        assertNull(fixture.requests.last().header("Cookie"))
    }

    @Test fun `streamed response body is read and closed off the UI caller thread`() = runBlocking {
        val reads = java.util.Collections.synchronizedList(mutableListOf<String>())
        val closed = AtomicBoolean()
        val fixture = Fixture { request, index ->
            val result = response(request, if (index == 0) login().toString() else preview().toString(), setCookie = if (index == 0) listOf(cookie) else emptyList())
            if (index == 0) result else {
                val bytes = Buffer().writeUtf8(preview().toString())
                val streaming = object : Source {
                    override fun read(sink: Buffer, byteCount: Long): Long {
                        reads.add(Thread.currentThread().name)
                        assertFalse(Thread.currentThread().name.startsWith("enterprise-test-ui"))
                        return bytes.read(sink, byteCount)
                    }
                    override fun timeout(): Timeout = Timeout.NONE
                    override fun close() { closed.set(true) }
                }.buffer()
                result.newBuilder().body(object : ResponseBody() {
                    override fun contentType() = "application/json".toMediaType()
                    override fun contentLength(): Long = -1
                    override fun source(): BufferedSource = streaming
                }).build()
            }
        }
        Executors.newSingleThreadExecutor { Thread(it, "enterprise-test-ui") }.asCoroutineDispatcher().use { ui ->
            withContext(ui) {
                val binding = fixture.open()
                assertTrue(fixture.gateway.preview(binding, code) is EnterpriseAccessResult.Ok)
            }
        }
        assertTrue(reads.isNotEmpty())
        assertTrue(closed.get())
    }

    @Test fun `invalidation cancels an owned call even after headers while streaming body`() = runBlocking {
        val entered = CountDownLatch(1)
        val release = CountDownLatch(1)
        val closed = AtomicBoolean()
        lateinit var fixture: Fixture
        fixture = Fixture { request, index ->
            if (index == 0) response(request, login().toString(), setCookie = listOf(cookie)) else {
                val body = object : ResponseBody() {
                    private val stream = object : Source {
                        override fun read(sink: Buffer, byteCount: Long): Long {
                            entered.countDown()
                            check(release.await(5, TimeUnit.SECONDS))
                            if (fixture.receivedCalls.last().isCanceled()) throw IOException("Synthetic cancellation")
                            error("An invalidated body must not remain live")
                        }
                        override fun timeout(): Timeout = Timeout.NONE
                        override fun close() { closed.set(true) }
                    }.buffer()
                    override fun contentType() = "application/json".toMediaType()
                    override fun contentLength(): Long = -1
                    override fun source(): BufferedSource = stream
                }
                response(request, "{}").newBuilder().body(body).build()
            }
        }
        val binding = fixture.open()
        val pending = async(Dispatchers.Default) { fixture.gateway.preview(binding, code) }
        assertTrue(entered.await(5, TimeUnit.SECONDS))
        fixture.gateway.invalidate()
        assertTrue(fixture.receivedCalls.last().isCanceled())
        release.countDown()
        assertError(EnterpriseAccessError.STALE, pending.await())
        assertTrue(closed.get())
    }

    @Test fun `overlapping opens cancel the older lease and only the latest cookie can authorize posts`() = runBlocking {
        val entered = CountDownLatch(1)
        val release = CountDownLatch(1)
        val latestCookie = cookie.replace("abv_company_${"c".repeat(64)}", "abv_company_${"f".repeat(64)}")
        val fixture = Fixture { request, index -> when (index) {
            0 -> { entered.countDown(); check(release.await(5, TimeUnit.SECONDS)); response(request, login().toString(), setCookie = listOf(cookie)) }
            1 -> response(request, login().toString(), setCookie = listOf(latestCookie))
            else -> response(request, preview().toString())
        } }
        val older = async(Dispatchers.Default) { fixture.gateway.open() }
        assertTrue(entered.await(5, TimeUnit.SECONDS))
        val latest = fixture.open()
        assertTrue(fixture.receivedCalls.first().isCanceled())
        release.countDown()
        assertError(EnterpriseAccessError.STALE, older.await())
        assertTrue(fixture.gateway.preview(latest, code) is EnterpriseAccessResult.Ok)
        assertEquals(latestCookie.substringBefore(';'), fixture.requests.last().header("Cookie"))
    }

    @OptIn(kotlinx.coroutines.ExperimentalCoroutinesApi::class)
    @Test fun `prompt cancellation closes response queued for a suspended consumer`(): Unit = runTest {
        val closed = AtomicBoolean()
        val request = Request.Builder().url(origin).build()
        val body = object : ResponseBody() {
            private val stream = object : Source {
                override fun read(sink: Buffer, byteCount: Long): Long = -1
                override fun timeout(): Timeout = Timeout.NONE
                override fun close() { closed.set(true) }
            }.buffer()
            override fun contentType() = "application/json".toMediaType()
            override fun contentLength(): Long = 0
            override fun source(): BufferedSource = stream
        }
        var callback: Callback? = null
        var cancelled = false
        val call = object : Call {
            override fun request(): Request = request
            override fun execute(): Response = error("Not a synchronous call")
            override fun enqueue(responseCallback: Callback) { callback = responseCallback }
            override fun cancel() { cancelled = true }
            override fun isExecuted(): Boolean = callback != null
            override fun isCanceled(): Boolean = cancelled
            override fun timeout(): Timeout = Timeout.NONE
            override fun clone(): Call = error("No clone")
        }
        val consumer = async { awaitCompanyResponse(call).use { error("Cancelled consumer must not receive response") } }
        runCurrent()
        callback!!.onResponse(call, response(request, "{}").newBuilder().body(body).build())
        consumer.cancel()
        runCurrent()
        assertTrue(cancelled)
        assertTrue(closed.get())
    }
}
