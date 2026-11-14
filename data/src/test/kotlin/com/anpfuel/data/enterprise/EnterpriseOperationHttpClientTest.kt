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
import io.mockk.verify
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

class EnterpriseOperationHttpClientTest {
    init {
        val tests = EnterpriseOperationHttpClientTest::class.java.declaredMethods.filter { it.isAnnotationPresent(Test::class.java) }
        check(tests.size == 10 && tests.all { it.returnType == Void.TYPE }) { "Queue HTTP tests must all be discoverable void methods" }
    }

    private val account = "7b0e2a54-0000-4000-8000-000000000001"
    private val organization = "7b0e2a54-0000-4000-8000-000000000002"
    private val member = "7b0e2a54-0000-4000-8000-000000000003"
    private val enrollment = "7b0e2a54-0000-4000-8000-000000000004"
    private val operationId = "7b0e2a54-0000-4000-8000-000000000005"
    private val other = "7b0e2a54-0000-4000-8000-000000000006"
    private val origin = "https://api.example.invalid"
    private val partition = EnterprisePartition.of(account, origin, organization)

    private fun operation(id: String = operationId, org: String = organization): EnterpriseOperation = EnterpriseOperation.seal(
        """{"schema_version":1,"operation_id":"$id","owner_account_id":"$account","api_origin":"$origin","org_id":"$org","membership_id":"$member","enrollment_id":"$enrollment","type":"validation.queue","operation_schema":1,"entity":{"type":"record","id":"$other","expected_revision":null},"occurred_at":"2026-11-13T12:00:00Z","entry_zone":"UTC","device_offset_seconds":0,"depends_on":[],"soft_references":[],"attachments":[],"payload":{"value":"Synthetic private queue fact"}}""".toByteArray(),
        SyncBinding.of(account, origin, org),
    )

    private fun acknowledgement(op: EnterpriseOperation, retry: Boolean = false) = JSONObject()
        .put("schema_version", 1).put("operation_id", op.operationId).put("payload_digest", op.payloadDigest)
        .put("result", if (retry) "RETRY" else "ACCEPTED").put("code", if (retry) "UNAVAILABLE" else JSONObject.NULL)
        .put("entity_revision", if (retry) JSONObject.NULL else 2).put("review_item_id", JSONObject.NULL)
        .put("received_at", if (retry) JSONObject.NULL else "2026-11-13T12:01:00Z").put("warnings", org.json.JSONArray())
    private fun answer(vararg values: JSONObject): String = "{\"schema_version\":1,\"acknowledgements\":[${values.joinToString(",")}]}"
    private fun response(request: Request, body: String, status: Int = 200, headers: Map<String, String> = emptyMap()): Response =
        Response.Builder().request(request).protocol(Protocol.HTTP_1_1).code(status).message("Synthetic queue response")
            .header("Cache-Control", "no-store").header("Content-Type", "application/json")
            .body(body.toResponseBody("application/json".toMediaType()))
            .apply { headers.forEach { (key, value) -> header(key, value) } }.build()

    private inner class Fixture(handle: (Request) -> Response) {
        val fence = EnterpriseSessionFence()
        @Volatile var stored: PortableAuth.Session? = PortableAuth.Session(account, account, "synthetic-access", "synthetic-refresh", 2000, 4000)
        var onLoad: () -> Unit = {}
        val sessions = object : AuthSessionStore {
            override fun load(): PortableAuth.Session? {
                val observed = stored
                onLoad()
                return observed
            }
            override fun save(session: PortableAuth.Session) { stored = session }
            override fun clear() { stored = null }
        }
        val auth = mockk<AuthFlow>().also { every { it.refreshSession() } answers { AuthApiResult.Ok(stored) } }
        var onSign: () -> Unit = {}
        val signed = mutableListOf<ByteArray>()
        val requests = mutableListOf<Request>()
        val source = EnterpriseOperationSessionSource(origin, sessions, auth, fence,
            { lease, selected ->
                lease.assertCurrent()
                check(selected == partition)
                EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(selected, member), enrollment, 1)
            }, { operationId })
        val client = EnterpriseOperationHttpClient(OkHttpClient.Builder().addInterceptor(Interceptor { chain ->
            requests += chain.request()
            handle(chain.request())
        }).build(), source, EnterpriseDeviceSigner { binding, path, bytes ->
            assertEquals(EnterpriseAccessBinding(account, origin, account, fence.epoch), binding)
            assertEquals("/v1/orgs/$organization/sync/operations", path)
            signed += bytes.copyOf()
            onSign()
            mapOf("Signature" to "synthetic-signature", "Signature-Created" to "1794571200",
                "Signature-Expires" to "1794571320", "Signature-Keyid" to "synthetic-key", "Signature-Nonce" to "synthetic-nonce")
        })
        suspend fun attempt(): EnterpriseQueueAttempt = source.attempt(source.deliveryAccount(), partition)
    }

    @Test fun `exact captured proof signs immutable operations and decodes the complete acknowledgement answer`(): Unit = runBlocking {
        val first = operation()
        val second = operation(other)
        val fixture = Fixture { response(it, answer(acknowledgement(first), acknowledgement(second, retry = true))) }
        val result = fixture.client.deliver(fixture.attempt(), listOf(first, second))
        assertEquals(listOf(SyncResult.ACCEPTED, SyncResult.RETRY), result.acknowledgements.map { it.result })
        val sent = fixture.requests.single()
        assertEquals(origin, sent.header("Origin"))
        for (header in listOf("Cookie", "Authorization", "X-CSRF", "Signature-Input")) assertNull(sent.header(header))
        val raw = Buffer().also { sent.body!!.writeTo(it) }.readByteArray()
        assertArrayEquals(fixture.signed.single(), raw)
        val batch = SyncBatch.parse(raw)
        assertEquals("synthetic-access", batch.accessToken)
        assertEquals(account, batch.familyId)
        assertArrayEquals(first.canonicalBytes, batch.operations.first().parse(first.binding).canonicalBytes)
        assertFalse(result.toString().contains(operationId))
    }

    @Test fun `foreign protected tuple is rejected before signing or network`(): Unit = runBlocking {
        val fixture = Fixture { error("foreign operation reached network") }
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking {
            fixture.client.deliver(fixture.attempt(), listOf(operation(org = other)))
        } }
        assertTrue(fixture.signed.isEmpty()); assertTrue(fixture.requests.isEmpty())
    }

    @Test fun `whole malformed answer is refused without returning a valid private prefix`(): Unit = runBlocking {
        val first = operation()
        val second = operation(other)
        val good = acknowledgement(first)
        val badAnswers = listOf(
            answer(good), answer(good, good), answer(acknowledgement(second), good),
            answer(good, acknowledgement(second).put("payload_digest", "sha256:" + "0".repeat(64))),
            answer(good, acknowledgement(second).put("final", true)),
            answer(good, acknowledgement(second)) + "{}",
            answer(good, acknowledgement(second)).replace("\"schema_version\":1", "\"schema_version\":1,\"schema_version\":1"),
        )
        for (body in badAnswers) {
            val fixture = Fixture { response(it, body) }
            assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking {
                fixture.client.deliver(fixture.attempt(), listOf(first, second))
            } }
            assertEquals(1, fixture.requests.size)
        }
    }

    @Test fun `signing and network cannot adopt background account family or token replacement`(): Unit = runBlocking {
        val op = operation()
        val mutations: List<(Fixture) -> Unit> = listOf(
            { it.fence.markBackground() }, { it.fence.lock() }, { it.stored = it.stored!!.copy(accountId = other) },
            { it.stored = it.stored!!.copy(familyId = other) }, { it.stored = it.stored!!.copy(accessToken = "replacement-access") },
        )
        for (change in mutations) {
            lateinit var fixture: Fixture
            fixture = Fixture { change(fixture); response(it, answer(acknowledgement(op))) }
            assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { fixture.client.deliver(fixture.attempt(), listOf(op)) } }
            assertEquals(1, fixture.requests.size)
        }
        val fixture = Fixture { error("stale signer reached network") }
        fixture.onSign = { fixture.fence.lock() }
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { fixture.client.deliver(fixture.attempt(), listOf(op)) } }
        assertTrue(fixture.requests.isEmpty())
    }

    @Test fun `refresh may replace a same family token only before a fresh account lease is captured`(): Unit = runBlocking {
        val op = operation()
        val fixture = Fixture { response(it, answer(acknowledgement(op))) }
        every { fixture.auth.refreshSession() } answers {
            fixture.stored = fixture.stored!!.copy(accessToken = "refreshed-access")
            AuthApiResult.Ok(fixture.stored)
        }
        fixture.client.deliver(fixture.attempt(), listOf(op))
        val raw = Buffer().also { fixture.requests.single().body!!.writeTo(it) }.readByteArray()
        assertEquals("refreshed-access", SyncBatch.parse(raw).accessToken)
        every { fixture.auth.refreshSession() } answers {
            fixture.stored = fixture.stored!!.copy(familyId = other)
            AuthApiResult.Ok(fixture.stored)
        }
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { fixture.source.deliveryAccount() } }
    }

    @Test fun `offline source capture does not refresh or require an unexpired personal access token`(): Unit = runBlocking {
        val fixture = Fixture { error("offline capture reached transport") }
        every { fixture.auth.refreshSession() } returns AuthApiResult.Err("unavailable")
        val accountLease = fixture.source.account()
        val attempt = fixture.source.attempt(accountLease, partition)
        assertEquals(EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(partition, member), enrollment, 1), attempt.enrollment)
        attempt.assertCurrent()
        verify(exactly = 0) { fixture.auth.refreshSession() }
        assertTrue(fixture.requests.isEmpty())
        assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { fixture.source.deliveryAccount() } }
    }

    @Test fun `generic denials cookies cache redirects and oversize never produce erasure or acknowledgements`(): Unit = runBlocking {
        val op = operation()
        val cases = listOf(
            Triple("{}", 401, emptyMap()), Triple("{}", 403, emptyMap()), Triple("{}", 404, emptyMap()),
            Triple("{}", 503, emptyMap()), Triple(answer(acknowledgement(op)), 302, mapOf("Location" to "https://foreign.example.invalid")),
            Triple(answer(acknowledgement(op)), 200, mapOf("Set-Cookie" to "synthetic=unexpected")),
            Triple(answer(acknowledgement(op)), 200, mapOf("Cache-Control" to "public")),
            Triple(" ".repeat(65537), 200, emptyMap()),
        )
        for ((body, status, headers) in cases) {
            val fixture = Fixture { response(it, body, status, headers) }
            assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { fixture.client.deliver(fixture.attempt(), listOf(op)) } }
            assertEquals(1, fixture.requests.size)
        }
    }

    @Test fun `retry after is honored without overflow or shortening and invalid values are ignored`(): Unit = runBlocking {
        val op = operation()
        for ((header, expected) in listOf("120" to 120000L, "9223372036854775807" to Long.MAX_VALUE,
            "Thu, 1 Jan 1970 00:02:00 GMT" to 120000L, "-1" to null, "invalid" to null)) {
            val fixture = Fixture { response(it, "{}", 429, mapOf("Retry-After" to header,
                "Date" to "Thu, 1 Jan 1970 00:00:00 GMT")) }
            val failure = assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { fixture.client.deliver(fixture.attempt(), listOf(op)) } }
            assertEquals(expected, failure.retryAfterMillis)
        }
    }

    @Test fun httpDateRetryAfterUsesAuthenticatedDateAndIgnoresSkew(): Unit = runBlocking {
        val op = operation()
        for (kind in listOf("valid", "missing", "invalid", "duplicate")) {
            val fixture = Fixture {
                val headers = mutableMapOf("Retry-After" to "Thu, 1 Jan 1970 00:02:00 GMT")
                if (kind != "missing") headers["Date"] = if (kind == "invalid") "invalid" else "Thu, 1 Jan 1970 00:00:00 GMT"
                val base = response(it, "{}", 429, headers)
                if (kind == "duplicate") base.newBuilder().addHeader("Date", "Thu, 1 Jan 1970 00:00:00 GMT").build() else base
            }
            val failure = assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { fixture.client.deliver(fixture.attempt(), listOf(op)) } }
            assertEquals(if (kind == "valid") 120_000L else null, failure.retryAfterMillis)
        }
    }

    @Test fun sourceLeaseRefusesBackgroundInvalidationDuringItsFinalSessionRead(): Unit = runBlocking {
        val fixture = Fixture { error("Source-only lease validation reached transport") }
        val lease = fixture.source.account()
        fixture.onLoad = {
            fixture.onLoad = {}
            // The provider has already observed the original session. A
            // background transition while that read finishes cannot validate
            // the captured old epoch merely because its token is unchanged.
            fixture.fence.markBackground()
        }
        assertThrows(EnterpriseLocalDenied::class.java) { lease.assertCurrent() }
        assertEquals(1L, fixture.fence.backgroundEpoch)
        assertTrue(fixture.signed.isEmpty()); assertTrue(fixture.requests.isEmpty())
        verify(exactly = 0) { fixture.auth.refreshSession() }
    }
}
