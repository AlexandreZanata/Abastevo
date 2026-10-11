package com.anpfuel.data.remote

import com.anpfuel.domain.repository.TrainingConsentException
import com.anpfuel.domain.repository.TrainingConsentReceipt
import com.anpfuel.domain.repository.TrainingConsentRejectKind
import com.anpfuel.domain.repository.TrainingConsentStatus
import kotlinx.coroutines.test.runTest
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

    private val server = MockWebServer()

    @AfterEach
    fun tearDown() {
        server.shutdown()
    }

    private fun client(): TrainingConsentHttpClient =
        TrainingConsentHttpClient(OkHttpClient(), server.url("/").toString())

    @Test
    fun `grant parses receipt`() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"state":"GRANTED","revision":1,"terms_version":"training-terms-v1","decided_at":"2026-10-11T00:00:00Z","replayed":false}""",
            ),
        )
        val receipt = client().grantConsent("training-terms-v1", "digest-v1", 1)
        assertEquals("GRANTED", receipt.state)
        assertEquals(1, receipt.revision)
        assertFalse(receipt.replayed)
        assertFalse(receipt.deleteOwnedTrainingAssets)
        val recorded = server.takeRequest()
        assertEquals("/v1/privacy/training-consent/grant", recorded.path)
        assertTrue(recorded.body.readUtf8().contains("\"revision\":1"))
    }

    @Test
    fun `replay converges with flag`() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"state":"GRANTED","revision":1,"terms_version":"training-terms-v1","decided_at":"2026-10-11T00:00:00Z","replayed":true}""",
            ),
        )
        val receipt = client().grantConsent("training-terms-v1", "digest-v1", 1)
        assertTrue(receipt.replayed)
    }

    @Test
    fun `revoke carries deletion signal`() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"state":"REVOKED","revision":2,"terms_version":"training-terms-v1","decided_at":"2026-10-11T00:00:00Z","replayed":false,"delete_owned_training_assets":true}""",
            ),
        )
        val receipt = client().revokeConsent(2)
        assertEquals("REVOKED", receipt.state)
        assertTrue(receipt.deleteOwnedTrainingAssets)
        val recorded = server.takeRequest()
        assertEquals("/v1/privacy/training-consent/revoke", recorded.path)
    }

    @Test
    fun `status parses granted view`() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"granted":true,"state":"GRANTED","terms_version":"training-terms-v1","terms_digest":"digest-v1","revision":1,"decided_at":"2026-10-11T00:00:00Z"}""",
            ),
        )
        val status = client().consentStatus()
        assertTrue(status.granted)
        assertEquals("GRANTED", status.state)
        assertEquals("digest-v1", status.termsDigest)
        assertEquals(1, status.revision)
        assertFalse(status.needsRegrant)
    }

    @Test
    fun `status parses default off`() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"granted":false}"""),
        )
        val status = client().consentStatus()
        assertFalse(status.granted)
        assertNull(status.state)
        assertEquals(0, status.revision)
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
            server.enqueue(MockResponse().setResponseCode(code).setBody(body))
            try {
                kotlinx.coroutines.test.runTest {
                    client().grantConsent("training-terms-v1", "digest-v1", 1)
                }
                fail("HTTP $code accepted")
            } catch (error: TrainingConsentException) {
                assertEquals(kind, error.kind, "HTTP $code")
            }
        }
    }

    @Test
    fun `unknown statuses and empty bodies never claim consent`() = runTest {
        server.enqueue(MockResponse().setResponseCode(500).setBody("boom"))
        try {
            client().consentStatus()
            fail("HTTP 500 accepted")
        } catch (error: TrainingConsentException) {
            assertEquals(TrainingConsentRejectKind.TRANSPORT, error.kind)
        }
        server.enqueue(MockResponse().setResponseCode(200).setBody(""))
        try {
            client().consentStatus()
            fail("empty body accepted")
        } catch (error: TrainingConsentException) {
            assertEquals(TrainingConsentRejectKind.TRANSPORT, error.kind)
        }
        server.enqueue(MockResponse().setResponseCode(200).setBody("not json"))
        try {
            client().revokeConsent(1)
            fail("malformed body accepted")
        } catch (error: TrainingConsentException) {
            assertEquals(TrainingConsentRejectKind.TRANSPORT, error.kind)
        }
    }

    @Test
    fun `blank inputs refuse before io`() = runTest {
        try {
            client().grantConsent("", "digest-v1", 1)
            fail("blank terms accepted")
        } catch (error: IllegalArgumentException) {
            assertTrue(error.message!!.contains("terms_version"))
        }
        assertEquals(0, server.requestCount)
    }
}
