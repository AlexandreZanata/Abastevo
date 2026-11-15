package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.attachment.AttachmentSessionResult
import com.anpfuel.application.enterprise.attachment.AttachmentTransferResult
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.enterprise.attachment.EnterpriseAttachmentHttpClient
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.attachment.*
import com.anpfuel.domain.enterprise.document.DocumentTargetKind
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
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseAttachmentHttpClientTest {
    private val account = "7b0e2a54-0000-4000-8000-000000000001"
    private val organization = "7b0e2a54-0000-4000-8000-000000000002"
    private val member = "7b0e2a54-0000-4000-8000-000000000003"
    private val enrollment = "7b0e2a54-0000-4000-8000-000000000004"
    private val requestId = "7b0e2a54-0000-4000-8000-000000000009"
    private val origin = "https://api.example.invalid"
    private val partition = EnterprisePartition.of(account, origin, organization)
    private val item = EnterpriseAttachment("7b0e2a54-0000-4000-8000-000000000010", "7b0e2a54-0000-4000-8000-000000000011",
        "7b0e2a54-0000-4000-8000-000000000012", "7b0e2a54-0000-4000-8000-000000000013", AttachmentTarget(DocumentTargetKind.INCIDENT,
            "7b0e2a54-0000-4000-8000-000000000014"), "ab".repeat(32), 1024, Instant.parse("2026-11-15T12:00:00Z"))
    private val bytes = ByteArray(1024) { 7 }

    private fun sessionAnswer(request: Request, storage: String = "UPLOADING", revision: Long = 1, rewrite: (String) -> String = { it }): String {
        val sent = AttachmentSessionRequest.parse(Buffer().also { request.body!!.writeTo(it) }.readByteArray()).snapshot()
        val upload = if (storage == "UPLOADING") """{"capability":"v1.synthetic-capability","expires_at":"2026-11-15T12:05:00Z"}""" else "null"
        return rewrite("""{"schema_version":1,"request_id":"${sent.requestId}","membership_id":"${sent.membershipId}","enrollment_id":"${sent.enrollmentId}","document_id":"${sent.documentId}","storage":"$storage","storage_revision":$revision,"upload":$upload}""")
    }
    private fun ingressAnswer(document: String = item.documentId, storage: String = "STORED", revision: Long = 3) =
        """{"document":{"id":"$document","kind":"RECEIPT","data_mode":"SYNTHETIC","original":{"sha256":"${"ab".repeat(32)}","bytes":1024,"media_type":"image/jpeg"},"statuses":{"storage":"$storage","extraction":"NONE","structure":"NOT_APPLICABLE","conference":"UNCONFERRED","reconciliation":"UNALLOCATED","fiscal":"NOT_CONSULTED"},"revisions":{"storage":$revision,"extraction":1,"structure":1,"conference":1,"fiscal":1},"created_at":"2026-11-15T12:00:00Z","updated_at":"2026-11-15T12:00:01Z"},"replayed":false}"""
    private fun error(code: String) = """{"error":{"code":"$code","message":"Organization request unavailable or refused","details":[]}}"""
    private fun response(request: Request, body: String, status: Int = 200, headers: Map<String, String> = emptyMap()): Response =
        Response.Builder().request(request).protocol(Protocol.HTTP_1_1).code(status).message("Synthetic attachment response")
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
        val source = EnterpriseOperationSessionSource(origin, sessions, auth, fence, { lease, selected ->
            lease.assertCurrent(); check(selected == partition)
            EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(selected, member), enrollment, 1)
        }, { requestId })
        val client = EnterpriseAttachmentHttpClient(OkHttpClient.Builder().addInterceptor(Interceptor { chain ->
            requests += chain.request(); handle(chain.request())
        }).build(), source, EnterpriseDeviceSigner { binding, path, raw ->
            assertEquals(EnterpriseAccessBinding(account, origin, account, fence.epoch), binding)
            signed += path to raw.copyOf()
            mapOf("Signature" to "synthetic-signature", "Signature-Created" to "1794571200",
                "Signature-Expires" to "1794571320", "Signature-Keyid" to "synthetic-key", "Signature-Nonce" to "synthetic-nonce")
        }, { requestId })
        suspend fun attempt(): EnterpriseQueueAttempt = source.attempt(source.deliveryAccount(), partition)
    }

    @Test fun signedSessionCarriesTheExactDeclarationAndReturnsTheBoundGrant(): Unit = runBlocking {
        val fixture = Fixture { response(it, sessionAnswer(it)) }
        val opened = fixture.client.open(fixture.attempt(), item) as AttachmentSessionResult.Opened
        assertEquals(AttachmentStorage.UPLOADING, opened.response.storage)
        assertEquals("v1.synthetic-capability", opened.response.upload!!.capability)
        val sent = fixture.requests.single()
        assertEquals("$origin/v1/orgs/$organization/sync/attachments", sent.url.toString())
        assertEquals("POST", sent.method); assertEquals(origin, sent.header("Origin")); assertEquals("no-store", sent.header("Cache-Control"))
        for (header in listOf("Cookie", "Authorization", "X-CSRF", "Signature-Input")) assertNull(sent.header(header))
        val raw = Buffer().also { sent.body!!.writeTo(it) }.readByteArray()
        val (path, signedBytes) = fixture.signed.single()
        assertEquals("/v1/orgs/$organization/sync/attachments", path); assertArrayEquals(signedBytes, raw)
        val request = AttachmentSessionRequest.parse(raw).snapshot()
        assertEquals(AttachmentRequestSnapshot(requestId, account, "synthetic-access", member, enrollment, item.declarationId, item.documentId,
            "RECEIPT", item.sha256, item.bytes, "image/jpeg", item.linkId, "INCIDENT", item.target.id), request)
        assertFalse(opened.toString().contains("synthetic-capability"))
    }

    @Test fun finalRefusalsAreClosedCodesAndEverythingElseIsRetried(): Unit = runBlocking {
        for ((status, code, refusal) in listOf(Triple(403, "FORBIDDEN", AttachmentRefusal.FORBIDDEN), Triple(409, "CONFLICT", AttachmentRefusal.CONFLICT),
            Triple(409, "OPERATION_ID_REUSED", AttachmentRefusal.OPERATION_ID_REUSED), Triple(422, "VALIDATION_FAILED", AttachmentRefusal.VALIDATION_FAILED))) {
            val fixture = Fixture { response(it, error(code), status) }
            assertEquals(refusal, (fixture.client.open(fixture.attempt(), item) as AttachmentSessionResult.Refused).refusal)
        }
        val retryable = listOf(404 to "NOT_FOUND", 401 to "UNAUTHENTICATED", 400 to "MALFORMED", 503 to "UNAVAILABLE", 403 to "ORG_SUSPENDED", 409 to "UNKNOWN")
        for ((status, code) in retryable) {
            val fixture = Fixture { response(it, error(code), status) }
            assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { fixture.client.open(fixture.attempt(), item) } }
        }
        val limited = Fixture { response(it, error("RATE_LIMITED"), 429, mapOf("Retry-After" to "120")) }
        assertEquals(120_000L, assertThrows(EnterpriseQueueDeliveryFailure::class.java) {
            runBlocking { limited.client.open(limited.attempt(), item) } }.retryAfterMillis)
        val foreign = listOf<(Request) -> String>({ r -> sessionAnswer(r) { it.replace(requestId, "7b0e2a54-0000-4000-8000-000000000099") } },
            { r -> sessionAnswer(r) { it.replace(item.documentId, "7b0e2a54-0000-4000-8000-000000000098") } },
            { r -> sessionAnswer(r, storage = "STORED") { it.replace("\"upload\":null", "\"upload\":{\"capability\":\"x\",\"expires_at\":\"2026-11-15T12:05:00Z\"}") } })
        for (body in foreign) {
            val fixture = Fixture { response(it, body(it)) }
            assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { fixture.client.open(fixture.attempt(), item) } }
        }
        for (headers in listOf(mapOf("Set-Cookie" to "synthetic=unexpected"), mapOf("Cache-Control" to "public"))) {
            val fixture = Fixture { response(it, sessionAnswer(it), 200, headers) }
            assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { fixture.client.open(fixture.attempt(), item) } }
        }
    }

    @Test fun ingressCarriesOnlyTheCapabilityAndTheExactBytes(): Unit = runBlocking {
        val fixture = Fixture { response(it, ingressAnswer()) }
        val grant = AttachmentUploadGrant("v1.synthetic-capability", Instant.parse("2026-11-15T12:05:00Z"))
        val settled = fixture.client.upload(fixture.attempt(), item.documentId, grant, bytes) as AttachmentTransferResult.Settled
        assertEquals(AttachmentStorage.STORED, settled.storage); assertEquals(3L, settled.revision)
        val sent = fixture.requests.single()
        assertEquals("$origin/v1/orgs/$organization/documents/${item.documentId}/content/upload", sent.url.toString())
        assertEquals("v1.synthetic-capability", sent.header("X-Abastevo-Upload-Capability"))
        assertEquals("image/jpeg", sent.body!!.contentType().toString()); assertEquals(1024L, sent.body!!.contentLength())
        assertArrayEquals(bytes, Buffer().also { sent.body!!.writeTo(it) }.readByteArray())
        for (header in listOf("Cookie", "Authorization", "Origin", "Signature", "Signature-Keyid", "Signature-Nonce")) assertNull(sent.header(header))
        assertTrue(fixture.signed.isEmpty(), "the ingress is never signed")
        val received = Fixture { response(it, ingressAnswer(storage = "RECEIVED", revision = 2)) }
        assertEquals(AttachmentStorage.RECEIVED, (received.client.upload(received.attempt(), item.documentId, grant, bytes) as AttachmentTransferResult.Settled).storage)
        for ((status, code) in listOf(401 to "UNAUTHENTICATED", 404 to "NOT_FOUND", 409 to "CONFLICT", 422 to "VALIDATION_FAILED")) {
            val refused = Fixture { response(it, error(code), status) }
            assertTrue(refused.client.upload(refused.attempt(), item.documentId, grant, bytes) is AttachmentTransferResult.Rejected, "$status")
        }
        for (answer in listOf<(Request) -> Response>({ response(it, error("UNAVAILABLE"), 503) }, { response(it, ingressAnswer(document = item.linkId)) },
            { response(it, ingressAnswer(storage = "VERIFIED")) })) {
            val failing = Fixture(answer)
            assertThrows(EnterpriseQueueDeliveryFailure::class.java) { runBlocking { failing.client.upload(failing.attempt(), item.documentId, grant, bytes) } }
        }
    }

    @Test fun accountOrTokenReplacementDuringAnAttachmentRequestIsDenied(): Unit = runBlocking {
        val mutations: List<(Fixture) -> Unit> = listOf({ it.fence.markBackground() }, { it.fence.lock() },
            { it.stored = it.stored!!.copy(accessToken = "replacement-access") })
        for (change in mutations) {
            lateinit var fixture: Fixture
            fixture = Fixture { change(fixture); response(it, sessionAnswer(it)) }
            assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { fixture.client.open(fixture.attempt(), item) } }
            lateinit var uploading: Fixture
            uploading = Fixture { change(uploading); response(it, ingressAnswer()) }
            assertThrows(EnterpriseLocalDenied::class.java) { runBlocking {
                uploading.client.upload(uploading.attempt(), item.documentId, AttachmentUploadGrant("v1.c", Instant.parse("2026-11-15T12:05:00Z")), bytes) } }
        }
    }
}
