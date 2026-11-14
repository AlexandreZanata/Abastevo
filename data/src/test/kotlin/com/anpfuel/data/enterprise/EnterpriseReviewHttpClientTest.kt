package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.sync.*
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

class EnterpriseReviewHttpClientTest {
    init {
        val tests = EnterpriseReviewHttpClientTest::class.java.declaredMethods.filter { it.isAnnotationPresent(Test::class.java) }
        check(tests.size == 6 && tests.all { it.returnType == Void.TYPE }) { "Review HTTP tests must all be discoverable void methods" }
    }

    private val account = "7b0e2a54-0000-4000-8000-000000000001"
    private val organization = "7b0e2a54-0000-4000-8000-000000000002"
    private val member = "7b0e2a54-0000-4000-8000-000000000003"
    private val enrollment = "7b0e2a54-0000-4000-8000-000000000004"
    private val requestId = "7b0e2a54-0000-4000-8000-000000000009"
    private val origin = "https://api.example.invalid"
    private val partition = EnterprisePartition.of(account, origin, organization)
    private val digest = "sha256:" + "a".repeat(64)
    private fun selector(n: Int) = ReviewSelector("7b0e2a54-0000-4000-8000-0000000001%02d".format(n), digest)
    private fun expected(n: Int) = ReviewExpectation(selector(n), "7b0e2a54-0000-4000-8000-0000000002%02d".format(n))
    private fun resolution(n: Int, reason: String = "Synthetic private reason 😀") = ReviewResolution.create(ReviewResolutionSnapshot(
        selector(n).operationId, digest, expected(n).reviewItemId, "7b0e2a54-0000-4000-8000-0000000003%02d".format(n), 2, "REFUSED",
        reason, "2026-11-14T12:00:00Z", null))
    private val bound = ReviewSourceBinding(SyncBinding.of(account, origin, organization), member, enrollment)
    private fun answer(request: Request, results: (List<ReviewSelector>) -> List<ReviewResult>): String {
        val raw = Buffer().also { request.body!!.writeTo(it) }.readByteArray()
        val parsed = ReviewResultsRequest.parse(raw)
        return String(ReviewResultsResponse.create(parsed, bound, results(parsed.snapshot().selectors)).canonicalBytes)
    }
    /** Same request ID and source, but a missing suffix slot. */
    private fun partial(request: Request): String {
        val raw = Buffer().also { request.body!!.writeTo(it) }.readByteArray()
        val one = ReviewResultsRequest.create(ReviewResultsRequest.parse(raw).snapshot().let { it.copy(selectors = it.selectors.take(1)) })
        return String(ReviewResultsResponse.create(one, bound, listOf(ReviewResult(selector(1), resolution(1)))).canonicalBytes)
    }
    private fun response(request: Request, body: String, status: Int = 200, headers: Map<String, String> = emptyMap()): Response =
        Response.Builder().request(request).protocol(Protocol.HTTP_1_1).code(status).message("Synthetic review response")
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
        val client = EnterpriseReviewHttpClient(OkHttpClient.Builder().addInterceptor(Interceptor { chain ->
            requests += chain.request(); handle(chain.request())
        }).build(), source, EnterpriseDeviceSigner { binding, path, bytes ->
            assertEquals(EnterpriseAccessBinding(account, origin, account, fence.epoch), binding)
            signed += path to bytes.copyOf(); onSign()
            mapOf("Signature" to "synthetic-signature", "Signature-Created" to "1794571200",
                "Signature-Expires" to "1794571320", "Signature-Keyid" to "synthetic-key", "Signature-Nonce" to "synthetic-nonce")
        }, { requestId })
        suspend fun attempt(): EnterpriseQueueAttempt = source.attempt(source.deliveryAccount(), partition)
    }

    @Test fun signedRequestCarriesExactSelectorsAndReturnsTheFullyCorrelatedOrderedAnswer(): Unit = runBlocking {
        val fixture = Fixture { request -> response(request, answer(request) { listOf(ReviewResult(it[0], resolution(1)), ReviewResult(it[1], null)) }) }
        val delivery = fixture.client.results(fixture.attempt(), listOf(expected(1), expected(2)))
        assertEquals(listOf(selector(1), selector(2)), delivery.results.map { it.selector })
        assertArrayEquals(resolution(1).canonicalBytes, delivery.results[0].resolution!!.canonicalBytes)
        assertNull(delivery.results[1].resolution)
        val sent = fixture.requests.single()
        assertEquals("$origin/v1/orgs/$organization/sync/review-results", sent.url.toString())
        assertEquals("POST", sent.method); assertEquals(origin, sent.header("Origin"))
        for (header in listOf("Cookie", "Authorization", "X-CSRF", "Signature-Input")) assertNull(sent.header(header))
        val raw = Buffer().also { sent.body!!.writeTo(it) }.readByteArray()
        val (path, signedBytes) = fixture.signed.single()
        assertEquals("/v1/orgs/$organization/sync/review-results", path); assertArrayEquals(signedBytes, raw)
        val request = ReviewResultsRequest.parse(raw).snapshot()
        assertEquals(listOf(selector(1), selector(2)), request.selectors)
        assertEquals(member, request.membershipId); assertEquals(enrollment, request.enrollmentId)
        assertEquals("synthetic-access", request.accessToken); assertEquals(account, request.familyId); assertEquals(requestId, request.requestId)
        assertFalse(delivery.toString().contains("Synthetic private"))
    }

    @Test fun anyUncorrelatedOrPartialAnswerIsRefusedWithoutAPrefix(): Unit = runBlocking {
        val bodies: List<(Request) -> String> = listOf(
            { r -> partial(r) },
            { r -> answer(r) { listOf(ReviewResult(it[0], resolution(1)), ReviewResult(it[1], null)) }.replace(requestId, "7b0e2a54-0000-4000-8000-000000000099") },
            { r -> answer(r) { listOf(ReviewResult(it[0], resolution(1)), ReviewResult(it[1], null)) }.replace(enrollment, "7b0e2a54-0000-4000-8000-000000000098") },
            { r -> answer(r) { listOf(ReviewResult(it[0], resolution(1)), ReviewResult(it[1], null)) } + "{}" },
            { r -> answer(r) { listOf(ReviewResult(it[0], resolution(1)), ReviewResult(it[1], null)) }.replace(expected(1).reviewItemId, "7b0e2a54-0000-4000-8000-000000000097") },
        )
        for (body in bodies) {
            val fixture = Fixture { response(it, body(it)) }
            assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { fixture.client.results(fixture.attempt(), listOf(expected(1), expected(2))) } }
            assertEquals(1, fixture.requests.size)
        }
    }

    @Test fun completeWorstCaseUnicodeAnswerUsesTheReviewBoundButNotMore(): Unit = runBlocking {
        val many = (1..50).map(::expected)
        val fixture = Fixture { request -> response(request, answer(request) { selectors ->
            selectors.mapIndexed { index, value -> ReviewResult(value, resolution(index + 1, "😀".repeat(280))) } }) }
        val delivery = fixture.client.results(fixture.attempt(), many)
        assertEquals(50, delivery.results.size)
        val oversize = Fixture { response(it, " ".repeat(ReviewResultsResponse.MAX_BYTES + 1)) }
        assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { oversize.client.results(oversize.attempt(), many) } }
        val tooMany = Fixture { error("an over-limit selector list reached the network") }
        assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { tooMany.client.results(tooMany.attempt(), (1..51).map(::expected)) } }
        assertTrue(tooMany.requests.isEmpty() && tooMany.signed.isEmpty())
    }

    @Test fun denialsCookiesCacheRedirectsAndRetryAfterNeverBecomeResults(): Unit = runBlocking {
        val cases = listOf(
            Triple("{}", 401, emptyMap()), Triple("{}", 403, emptyMap()), Triple("{}", 404, emptyMap()), Triple("{}", 503, emptyMap()),
            Triple("{}", 302, mapOf("Location" to "https://foreign.example.invalid")),
            Triple("{}", 200, mapOf("Set-Cookie" to "synthetic=unexpected")), Triple("{}", 200, mapOf("Cache-Control" to "public")),
        )
        for ((body, status, headers) in cases) {
            val fixture = Fixture { response(it, body, status, headers) }
            assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { fixture.client.results(fixture.attempt(), listOf(expected(1))) } }
        }
        val limited = Fixture { response(it, "{}", 429, mapOf("Retry-After" to "120")) }
        val failure = assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { limited.client.results(limited.attempt(), listOf(expected(1))) } }
        assertEquals(120_000L, failure.retryAfterMillis)
        // The review lookup shares the bounded Retry-After profile (EDR-122): a huge hint cannot park review work.
        val parked = Fixture { response(it, "{}", 503, mapOf("Retry-After" to "9223372036854775807")) }
        val bounded = assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { parked.client.results(parked.attempt(), listOf(expected(1))) } }
        assertEquals(21_600_000L, bounded.retryAfterMillis)
    }

    @Test fun backgroundAccountOrTokenReplacementDuringLookupIsDenied(): Unit = runBlocking {
        val mutations: List<(Fixture) -> Unit> = listOf({ it.fence.markBackground() }, { it.fence.lock() },
            { it.stored = it.stored!!.copy(accessToken = "replacement-access") }, { it.stored = it.stored!!.copy(accountId = organization) })
        for (change in mutations) {
            lateinit var fixture: Fixture
            fixture = Fixture { change(fixture); response(it, answer(it) { s -> listOf(ReviewResult(s[0], null)) }) }
            assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { fixture.client.results(fixture.attempt(), listOf(expected(1))) } }
        }
        val fixture = Fixture { error("a stale signature reached the network") }
        fixture.onSign = { fixture.fence.lock() }
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { fixture.client.results(fixture.attempt(), listOf(expected(1))) } }
        assertTrue(fixture.requests.isEmpty())
    }

    @Test fun foreignPartitionOrDuplicateSelectorsAreRefusedBeforeSigning(): Unit = runBlocking {
        val fixture = Fixture { error("an invalid lookup reached the network") }
        assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { fixture.client.results(fixture.attempt(), listOf(expected(1), expected(1))) } }
        assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { fixture.client.results(fixture.attempt(), emptyList()) } }
        assertTrue(fixture.signed.isEmpty() && fixture.requests.isEmpty())
    }
}
