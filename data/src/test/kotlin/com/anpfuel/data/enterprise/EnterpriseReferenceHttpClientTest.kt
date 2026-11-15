package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.fleet.EnterpriseReferenceNotFound
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.enterprise.fleet.EnterpriseReferenceHttpClient
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.fleet.ReferenceFeedMode
import com.anpfuel.domain.enterprise.fleet.ReferenceFeedRequest
import com.anpfuel.domain.enterprise.fleet.ReferenceFeedResponse
import com.anpfuel.domain.portable.PortableAuth
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.runBlocking
import okhttp3.Interceptor
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Request
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import okio.Buffer
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseReferenceHttpClientTest {
    init {
        val tests = EnterpriseReferenceHttpClientTest::class.java.declaredMethods.filter { it.isAnnotationPresent(Test::class.java) }
        check(tests.size == 5 && tests.all { it.returnType == Void.TYPE }) { "Reference HTTP tests must all be discoverable void methods" }
    }

    private val account = "7b0e2a54-0000-4000-8000-000000000001"
    private val organization = "7b0e2a54-0000-4000-8000-000000000002"
    private val member = "7b0e2a54-0000-4000-8000-000000000003"
    private val enrollment = "7b0e2a54-0000-4000-8000-000000000004"
    private val requestId = "7b0e2a54-0000-4000-8000-000000000009"
    private val assetId = "7b0e2a54-0000-4000-8000-000000000030"
    private val origin = "https://api.example.invalid"
    private val partition = EnterprisePartition.of(account, origin, organization)
    private val asset = """{"id":"$assetId","revision":1,"kind":"VEHICLE","usage_basis":"DISTANCE","state":"ACTIVE","name":"Caminhão <privado>","fleet_number":"F-1","plate":"ABC1D23","catalog":"fuel-products-v1","products":["DIESEL_S10"],"meters":[],"assignment":{"kind":"PRIMARY","valid_to":null}}"""

    /** A server-shaped answer to the exact request that reached the network. */
    private fun answer(request: Request, mode: String = "RESET", from: Long? = null, revision: Long = 1, withAsset: Boolean = true,
        rewrite: (String) -> String = { it }): String {
        val raw = Buffer().also { request.body!!.writeTo(it) }.readByteArray()
        val sent = ReferenceFeedRequest.parse(raw).snapshot()
        val reason = if (mode == "RESET") "\"NEW_CURSOR\"" else "null"
        val changes = if (withAsset) """[{"revision":$revision,"entity_type":"asset","entity_id":"$assetId","op":"UPSERT","asset":$asset}]""" else "[]"
        return rewrite("""{"schema_version":1,"request_id":"${sent.requestId}","membership_id":"${sent.membershipId}","enrollment_id":"${sent.enrollmentId}","mode":"$mode","reason":$reason,"from_revision":${from ?: "null"},"revision":$revision,"confirmed_at":"2026-11-15T12:00:00.5Z","cursor":"next-cursor","entity_types":["asset"],"changes":$changes}""")
    }
    private fun response(request: Request, body: String, status: Int = 200, headers: Map<String, String> = emptyMap()): Response =
        Response.Builder().request(request).protocol(Protocol.HTTP_1_1).code(status).message("Synthetic reference response")
            .header("Cache-Control", "no-store").header("Content-Type", "application/json")
            .body(body.toResponseBody("application/json".toMediaType()))
            .apply { headers.forEach { (key, value) -> header(key, value) } }.build()

    private inner class Fixture(handle: (Request) -> Response) {
        val fence = EnterpriseSessionFence()
        @Volatile var stored: PortableAuth.Session? = PortableAuth.Session(account, account, "synthetic-access", "synthetic-refresh", 2000, 4000)
        val sessions = object : AuthSessionStore {
            override fun load(): PortableAuth.Session? = stored
            override fun save(session: PortableAuth.Session) { stored = session }
            override fun clear() { stored = null }
        }
        val auth = mockk<AuthFlow>().also { every { it.refreshSession() } answers { AuthApiResult.Ok(stored) } }
        val signed = mutableListOf<Pair<String, ByteArray>>()
        val requests = mutableListOf<Request>()
        var onSign: () -> Unit = {}
        val source = EnterpriseOperationSessionSource(origin, sessions, auth, fence, { lease, selected ->
            lease.assertCurrent(); check(selected == partition)
            EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(selected, member), enrollment, 1)
        }, { requestId })
        val client = EnterpriseReferenceHttpClient(OkHttpClient.Builder().addInterceptor(Interceptor { chain ->
            requests += chain.request(); handle(chain.request())
        }).build(), source, EnterpriseDeviceSigner { binding, path, bytes ->
            assertEquals(EnterpriseAccessBinding(account, origin, account, fence.epoch), binding)
            signed += path to bytes.copyOf(); onSign()
            mapOf("Signature" to "synthetic-signature", "Signature-Created" to "1794571200",
                "Signature-Expires" to "1794571320", "Signature-Keyid" to "synthetic-key", "Signature-Nonce" to "synthetic-nonce")
        }, { requestId })
        suspend fun attempt(): EnterpriseQueueAttempt = source.attempt(source.deliveryAccount(), partition)
    }

    @Test fun signedPullCarriesTheExactBindingAndCursorAndReturnsTheBoundResponse(): Unit = runBlocking {
        val fixture = Fixture { request -> response(request, answer(request, mode = "DELTA", from = 3, revision = 4)) }
        val pull = fixture.client.pull(fixture.attempt(), "held-cursor", 3)
        assertEquals(ReferenceFeedMode.DELTA, pull.response.mode); assertEquals(3L, pull.response.fromRevision)
        assertEquals("Caminhão <privado>", pull.response.changes.single().asset!!.name)
        val sent = fixture.requests.single()
        assertEquals("$origin/v1/orgs/$organization/sync/reference", sent.url.toString())
        assertEquals("POST", sent.method); assertEquals(origin, sent.header("Origin")); assertEquals("no-store", sent.header("Cache-Control"))
        for (header in listOf("Cookie", "Authorization", "X-CSRF", "Signature-Input")) assertNull(sent.header(header))
        val raw = Buffer().also { sent.body!!.writeTo(it) }.readByteArray()
        val (path, signedBytes) = fixture.signed.single()
        assertEquals("/v1/orgs/$organization/sync/reference", path); assertArrayEquals(signedBytes, raw)
        assertArrayEquals(pull.request.canonicalBytes, raw)
        val request = ReferenceFeedRequest.parse(raw).snapshot()
        assertEquals(member, request.membershipId); assertEquals(enrollment, request.enrollmentId); assertEquals("held-cursor", request.cursor)
        assertEquals("synthetic-access", request.accessToken); assertEquals(account, request.familyId); assertEquals(requestId, request.requestId)
        assertFalse(pull.toString().contains("privado") || pull.toString().contains("held-cursor"))
        val first = Fixture { request -> response(request, answer(request)) }
        assertNull(ReferenceFeedRequest.parse(Buffer().also {
            first.client.pull(first.attempt(), null, null); first.requests.single().body!!.writeTo(it) }.readByteArray()).snapshot().cursor)
    }

    @Test fun anyUncorrelatedMalformedOrGappedAnswerIsRefusedWhole(): Unit = runBlocking {
        val bodies: List<(Request) -> String> = listOf(
            { r -> answer(r) { it.replace(requestId, "7b0e2a54-0000-4000-8000-000000000099") } },
            { r -> answer(r) { it.replace(enrollment, "7b0e2a54-0000-4000-8000-000000000098") } },
            { r -> answer(r) + "{}" },
            { r -> answer(r) { it.replace("\"entity_types\":[\"asset\"]", "\"entity_types\":[\"asset\",\"trip\"]") } },
            { r -> answer(r, mode = "DELTA", from = 2, revision = 4) },
            { r -> answer(r, revision = 0) },
        )
        for (body in bodies) {
            val fixture = Fixture { response(it, body(it)) }
            assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { fixture.client.pull(fixture.attempt(), "held-cursor", 3) } }
            assertEquals(1, fixture.requests.size)
        }
    }

    @Test fun notFoundIsDistinctAndDenialsCookiesCacheAndRedirectsNeverBecomeData(): Unit = runBlocking {
        val missing = Fixture { response(it, "{}", 404) }
        assertThrows(EnterpriseReferenceNotFound::class.java) { runBlocking { missing.client.pull(missing.attempt(), "held-cursor", 3) } }
        val cases = listOf(
            Triple("{}", 401, emptyMap()), Triple("{}", 403, emptyMap()), Triple("{}", 503, emptyMap()),
            Triple("{}", 302, mapOf("Location" to "https://foreign.example.invalid")),
        )
        for ((body, status, headers) in cases) {
            val fixture = Fixture { response(it, body, status, headers) }
            assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { fixture.client.pull(fixture.attempt(), null, null) } }
        }
        for (headers in listOf(mapOf("Set-Cookie" to "synthetic=unexpected"), mapOf("Cache-Control" to "public"))) {
            val fixture = Fixture { response(it, answer(it), 200, headers) }
            assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { fixture.client.pull(fixture.attempt(), null, null) } }
        }
        val limited = Fixture { response(it, "{}", 429, mapOf("Retry-After" to "120")) }
        assertEquals(120_000L, assertThrows(EnterpriseQueueDeliveryFailure::class.java) {
            runBlocking { limited.client.pull(limited.attempt(), null, null) } }.retryAfterMillis)
    }

    @Test fun answersAboveTheReferenceBoundAreRefusedAndTheBoundHoldsAFullFeed(): Unit = runBlocking {
        val oversize = Fixture { response(it, " ".repeat(ReferenceFeedResponse.MAX_BYTES + 1)) }
        assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { oversize.client.pull(oversize.attempt(), null, null) } }
        // 200 assets at their OpenAPI maxima, every name character HTML-escaped by the server, still fit.
        val name = "\\u003c".repeat(80)
        val products = (1..64).joinToString(",") { "\"" + "X".repeat(62) + "%02d".format(it) + "\"" }
        val meters = (1..16).joinToString(",") { i ->
            """{"id":"7b0e2a54-0000-4000-8001-${"%012d".format(i)}","kind":"ODOMETER","modulus":9223372036854775807,"segment_id":"7b0e2a54-0000-4000-8002-${"%012d".format(i)}","installed_at":"2026-11-01T08:00:00.123456789Z"}""" }
        val full = Fixture { request ->
            val sent = ReferenceFeedRequest.parse(Buffer().also { request.body!!.writeTo(it) }.readByteArray()).snapshot()
            val changes = (1..200).joinToString(",") { i ->
                val id = "7b0e2a54-0000-4000-8003-${"%012d".format(i)}"
                """{"revision":$i,"entity_type":"asset","entity_id":"$id","op":"UPSERT","asset":{"id":"$id","revision":9223372036854775807,"kind":"IMPLEMENT","usage_basis":"DISTANCE","state":"ARCHIVED","name":"$name","fleet_number":"${"\\u0026".repeat(40)}","plate":"${"\\u003e".repeat(40)}","catalog":"fuel-products-v1","products":[$products],"meters":[$meters],"assignment":{"kind":"ADDITIONAL","valid_to":"2026-11-16T08:30:00.123456789Z"}}}""" }
            response(request, """{"schema_version":1,"request_id":"${sent.requestId}","membership_id":"${sent.membershipId}","enrollment_id":"${sent.enrollmentId}","mode":"RESET","reason":"NEW_CURSOR","from_revision":null,"revision":200,"confirmed_at":"2026-11-15T12:00:00.123456789Z","cursor":"${"k".repeat(1024)}","entity_types":["asset"],"changes":[$changes]}""")
        }
        assertEquals(200, full.client.pull(full.attempt(), null, null).response.changes.size)
    }

    @Test fun backgroundAccountOrTokenReplacementDuringPullIsDenied(): Unit = runBlocking {
        val mutations: List<(Fixture) -> Unit> = listOf({ it.fence.markBackground() }, { it.fence.lock() },
            { it.stored = it.stored!!.copy(accessToken = "replacement-access") }, { it.stored = it.stored!!.copy(accountId = organization) })
        for (change in mutations) {
            lateinit var fixture: Fixture
            fixture = Fixture { change(fixture); response(it, answer(it)) }
            assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { fixture.client.pull(fixture.attempt(), null, null) } }
        }
        val fixture = Fixture { error("a stale signature reached the network") }
        fixture.onSign = { fixture.fence.lock() }
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { fixture.client.pull(fixture.attempt(), null, null) } }
        assertTrue(fixture.requests.isEmpty())
    }
}
