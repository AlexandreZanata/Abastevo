package com.anpfuel.data.remote

import com.anpfuel.domain.repository.TrainingConsentException
import com.anpfuel.domain.repository.TrainingConsentReceipt
import com.anpfuel.domain.repository.TrainingConsentRejectKind
import com.anpfuel.domain.repository.TrainingConsentStatus
import kotlinx.coroutines.test.runTest
import io.mockk.*
import org.json.JSONObject
import com.anpfuel.domain.model.ContributionScope
import com.anpfuel.domain.repository.ContributionRateLimited
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Assertions.fail
import org.junit.jupiter.api.Test

/**
 * CS11A: training-consent HTTP mapping (B-BR-CS06).
 *
 * Receipts and the owner status parse exactly; frozen backend codes
 * surface as kinds; transports, empty bodies and unknown statuses
 * never claim consent. Request paths and revisions travel verbatim
 * for server-side ordering and convergence.
 */
class TrainingConsentHttpClientTest {

    private val scope = ContributionScope("account:owner:key", "https://test.invalid")
    private val proof = mockk<PhotoProofTransport>()
    private val responses = java.util.ArrayDeque<Pair<Int, String>>()
    private var calls = 0
    private var lastPath = ""
    private var lastBody = ""
    private fun enqueue(code: Int, body: String) { responses.add(code to body) }
    private fun respond(path: String, body: JSONObject? = null): JSONObject {
        calls++; lastPath = path; lastBody = body?.toString().orEmpty()
        val (code, raw) = responses.removeFirst()
        if (code !in 200..299) throw PhotoOperationRefused(code,
            runCatching { JSONObject(raw).getJSONObject("error").getString("code") }.getOrNull())
        return JSONObject(raw)
    }
    private fun client(): TrainingConsentHttpClient {
        every { proof.origin } returns scope.origin
        coEvery { proof.post(any(), any(), any(), scope.ownerScope) } coAnswers { respond(firstArg(), thirdArg()) }
        coEvery { proof.get(any(), scope.ownerScope) } coAnswers { respond(firstArg()) }
        return TrainingConsentHttpClient(proof)
    }

    @Test
    fun `grant parses receipt`() = runTest {
        enqueue(200, """{"state":"GRANTED","revision":1,"terms_version":"training-terms-v1","decided_at":"2026-10-11T00:00:00Z","replayed":false}""")
        val receipt = client().grantConsent(scope, "training-terms-v1", "digest-v1", 1, "decision")
        assertEquals("GRANTED", receipt.state)
        assertEquals(1L, receipt.revision)
        assertFalse(receipt.replayed)
        assertFalse(receipt.deleteOwnedTrainingAssets)
        assertEquals("/v1/privacy/training-consent/grant", lastPath)
        assertTrue(lastBody.contains("\"revision\":1"))
    }

    @Test
    fun `replay converges with flag`() = runTest {
        enqueue(200, """{"state":"GRANTED","revision":1,"terms_version":"training-terms-v1","decided_at":"2026-10-11T00:00:00Z","replayed":true}""")
        val receipt = client().grantConsent(scope, "training-terms-v1", "digest-v1", 1, "decision")
        assertTrue(receipt.replayed)
    }

    @Test
    fun `revoke carries deletion signal`() = runTest {
        enqueue(200, """{"state":"REVOKED","revision":2,"terms_version":"training-terms-v1","decided_at":"2026-10-11T00:00:00Z","replayed":false,"delete_owned_training_assets":true}""")
        val receipt = client().revokeConsent(scope, 2, "decision")
        assertEquals("REVOKED", receipt.state)
        assertTrue(receipt.deleteOwnedTrainingAssets)
        assertEquals("/v1/privacy/training-consent/revoke", lastPath)
    }

    @Test
    fun `status parses granted view`() = runTest {
        enqueue(200, """{"granted":true,"state":"GRANTED","terms_version":"training-terms-v1","terms_digest":"digest-v1","revision":1,"decided_at":"2026-10-11T00:00:00Z"}""")
        val status = client().consentStatus(scope)
        assertTrue(status.granted)
        assertEquals("GRANTED", status.state)
        assertEquals("digest-v1", status.termsDigest)
        assertEquals(1L, status.revision)
        assertFalse(status.needsRegrant)
    }

    @Test
    fun `status parses default off`() = runTest {
        enqueue(200, """{"granted":false}""")
        val status = client().consentStatus(scope)
        assertFalse(status.granted)
        assertNull(status.state)
        assertEquals(0L, status.revision)
    }

    @Test
    fun `frozen codes map to kinds`() {
        val cases = mapOf(
            Pair(401, """{"error":{"code":"privacy.auth-required"}}""") to TrainingConsentRejectKind.AUTH_REQUIRED,
            Pair(400, """{"error":{"code":"privacy.terms-outdated"}}""") to TrainingConsentRejectKind.TERMS_OUTDATED,
            Pair(409, """{"error":{"code":"privacy.stale-revision"}}""") to TrainingConsentRejectKind.STALE_REVISION,
            Pair(409, """{"error":{"code":"privacy.conflict"}}""") to TrainingConsentRejectKind.CONFLICT,
            Pair(400, """{"error":{"code":"privacy.bad-body"}}""") to TrainingConsentRejectKind.INVALID,
        )
        for ((request, kind) in cases) {
            val (code, body) = request
            enqueue(code, body)
            try {
                kotlinx.coroutines.test.runTest {
                    client().grantConsent(scope, "training-terms-v1", "digest-v1", 1, "decision")
                }
                fail("HTTP $code accepted")
            } catch (error: TrainingConsentException) {
                assertEquals(kind, error.kind, "HTTP $code")
            }
        }
    }

    @Test
    fun `unknown statuses and empty bodies never claim consent`() = runTest {
        enqueue(500, "boom")
        try {
            client().consentStatus(scope)
            fail("HTTP 500 accepted")
        } catch (error: TrainingConsentException) {
            assertEquals(TrainingConsentRejectKind.TRANSPORT, error.kind)
        }
        enqueue(200, "")
        try {
            client().consentStatus(scope)
            fail("empty body accepted")
        } catch (error: TrainingConsentException) {
            assertEquals(TrainingConsentRejectKind.TRANSPORT, error.kind)
        }
        enqueue(200, "not json")
        try {
            client().revokeConsent(scope, 1, "decision")
            fail("malformed body accepted")
        } catch (error: TrainingConsentException) {
            assertEquals(TrainingConsentRejectKind.TRANSPORT, error.kind)
        }
    }

    @Test
    fun `blank inputs refuse before io`() = runTest {
        try {
            client().grantConsent(scope, "", "digest-v1", 1, "decision")
            fail("blank terms accepted")
        } catch (error: IllegalArgumentException) {
            assertTrue(error.message!!.contains("terms_version"))
        }
        assertEquals(0, calls)
    }
    @Test fun `foreign origin and anonymous scope refuse before transport`() = runTest {
        val client = client()
        for (foreign in listOf(scope.copy(origin="https://other.invalid"), scope.copy(ownerScope="anonymous:key"))) {
            try { client.consentStatus(foreign); fail<Unit>("foreign accepted") }
            catch (e: TrainingConsentException) { assertEquals(TrainingConsentRejectKind.AUTH_REQUIRED,e.kind) }
        }
        coVerify(exactly=0) { proof.get(any(),any()) }
    }

    @Test fun `invalid state revision and acknowledgement refuse closed`() = runTest {
        for (body in listOf("{\"granted\":\"true\"}", "{\"granted\":true}",
            "{\"granted\":false,\"revision\":1.5}", "{\"granted\":false,\"state\":\"OTHER\"}")) {
            enqueue(200,body)
            try { client().consentStatus(scope); fail<Unit>("invalid accepted") }
            catch(e:TrainingConsentException) { assertEquals(TrainingConsentRejectKind.TRANSPORT,e.kind) }
        }
        enqueue(200,"{\"state\":\"REVOKED\",\"revision\":2,\"replayed\":false}")
        try { client().revokeConsent(scope,1,"decision"); fail<Unit>("mismatched receipt") }
        catch(e:TrainingConsentException) { assertEquals(TrainingConsentRejectKind.TRANSPORT,e.kind) }
    }

    @Test fun `rate limits preserve retry hint and cancellation propagates`() = runTest {
        val client=client()
        coEvery { proof.get(any(),any()) } throws ContributionRateLimited(7200)
        try { client.consentStatus(scope); fail<Unit>("rate limited accepted") }
        catch(e:TrainingConsentException) { assertEquals(7200L,e.retryAfterSeconds) }
        coEvery { proof.get(any(),any()) } throws kotlinx.coroutines.CancellationException()
        try { client.consentStatus(scope); fail<Unit>("cancel swallowed") }
        catch(_:kotlinx.coroutines.CancellationException) { }
    }

}
