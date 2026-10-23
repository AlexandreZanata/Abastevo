package com.anpfuel.data.remote.ledger

import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class LedgerSyncHttpClientTest {

    private fun okBody(revision: Long = 2, replayed: Boolean = false) =
        """{"results":[{"operation_id":"11111111-1111-4111-8111-111111111111","dataset":"refuel","id":"22222222-2222-4222-8222-222222222222","revision":$revision,"status":"accepted","replayed":$replayed}]}"""

    private val scope = com.anpfuel.domain.premium.PremiumOwnerScope("acct-1", "https://api.example.test")

    private fun command(
        operationId: String = "11111111-1111-4111-8111-111111111111",
        recordId: String = "22222222-2222-4222-8222-222222222222",
        payloadJson: String = "{}",
    ) = com.anpfuel.domain.premium.PremiumCommand(
        operationId = operationId, scope = scope, dataset = "refuel", recordId = recordId,
        expectedRevision = 0, payloadJson = payloadJson,
    )

    @Test fun `submit sends exact wire and parses receipts`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody(okBody()))
            val api = LedgerSyncHttpClient(OkHttpClient(), server.url("/").toString())
            val receipt = runBlocking {
                api.submit(scope, command(payloadJson = """{"a":1}"""), "attempt-1", "fam-1", "tok")
            }
            assertEquals(2L, receipt.revision)
            assertEquals(false, receipt.replayed)
            val request = server.takeRequest(1, TimeUnit.SECONDS)
            val body = request!!.body.readUtf8()
            assertTrue(body.contains("\"operation_id\":\"11111111-1111-4111-8111-111111111111\""))
            assertTrue(body.contains("\"family_id\":\"fam-1\""))
            assertTrue(body.contains("\"expected_revision\":0"))
            assertTrue(request.getHeader("Authorization") == null)
            assertEquals("no-store", request.getHeader("Cache-Control"))
        }
    }

    @Test fun `mismatched receipt and error codes map to typed failures`() = runTest {
        MockWebServer().use { server ->
            val api = LedgerSyncHttpClient(OkHttpClient(), server.url("/").toString())
            suspend fun send(): com.anpfuel.domain.repository.LedgerSyncReceipt =
                api.submit(scope, command(), "attempt-1", "f", "t")
            server.enqueue(MockResponse().setBody(okBody().replace("11111111-1111-4111-8111-111111111111", "99999999-9999-4999-8999-999999999999")))
            assertThrows(com.anpfuel.domain.repository.LedgerSyncDenied::class.java) {
                runBlocking { send() }
            }
            server.enqueue(MockResponse().setResponseCode(409).setBody("{\"error\":\"conflict\"}"))
            assertThrows(com.anpfuel.domain.repository.LedgerSyncConflict::class.java) {
                runBlocking { send() }
            }
            server.enqueue(MockResponse().setResponseCode(400).setBody("{\"error\":\"invalid\"}"))
            assertThrows(com.anpfuel.domain.repository.LedgerSyncConflict::class.java) {
                runBlocking { send() }
            }
            server.enqueue(MockResponse().setResponseCode(503).setBody("{\"error\":\"unavailable\"}"))
            assertThrows(java.io.IOException::class.java) {
                runBlocking { send() }
            }
            server.enqueue(MockResponse().setBody("x".repeat(1024 * 1024 + 1)))
            assertThrows(java.io.IOException::class.java) {
                runBlocking { send() }
            }
        }
    }

    private fun assertTrue(value: Boolean) = org.junit.jupiter.api.Assertions.assertTrue(value)
}
