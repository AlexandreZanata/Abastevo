package com.anpfuel.data.remote.ledger

import com.anpfuel.domain.repository.LedgerSyncDenied
import java.io.IOException
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class LedgerRecordsHttpClientTest {
    private val id = "11111111-1111-4111-8111-111111111111"
    private val vehicle = "22222222-2222-4222-8222-222222222222"
    private fun row() = JSONObject("""{"dataset":"refuel","id":"$id","vehicle_id":"$vehicle","occurred_at":"2026-03-15T10:30:00Z","revision":2,"voided":false,"payload":{"id":"$id","vehicle_id":"$vehicle","occurred_at":"2026-03-15T10:30:00Z","timezone":"UTC","fuel":"ETHANOL","quantity_milli":20000,"total_centavos":12400,"fill":"full"}}""")
    private fun page(row: JSONObject = row()) = """{"items":[$row],"next_cursor":"opaque+/cursor="}"""

    @Test fun `typed page preserves opaque cursors and posts private no-store proof`() = runBlocking {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody(page()))
            val api = LedgerSyncHttpClient(OkHttpClient(), server.url("/").toString())
            val result = api.list("family", "proof", "refuel", null, 100, "previous+/=")
            assertEquals(id, result.items.single().id)
            assertEquals(2L, result.items.single().revision)
            assertEquals("opaque+/cursor=", result.nextCursor)
            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertEquals("POST", request.method)
            assertEquals("/v1/ledger/records", request.path)
            assertEquals("no-store", request.getHeader("Cache-Control"))
            assertNull(request.getHeader("Authorization"))
            val body = JSONObject(request.body.readUtf8())
            assertEquals("previous+/=", body.getString("cursor"))
            assertEquals("proof", body.getString("access_token"))
        }
    }

    @Test fun `foreign malformed missing and negative revisions or payloads fail closed`() {
        MockWebServer().use { server ->
            val api = LedgerSyncHttpClient(OkHttpClient(), server.url("/").toString())
            val cases = listOf(
                row().put("dataset", "expense"), row().put("id", "bad"),
                row().apply { remove("revision") }, row().put("revision", -1),
                row().put("revision", "2"), row().put("revision", 1.5),
                row().put("payload", "corrupt"), row().put("payload", JSONObject()),
                row().put("voided", "false"),
                row().apply { getJSONObject("payload").put("id", vehicle) },
            )
            for (bad in cases) {
                server.enqueue(MockResponse().setBody(page(bad)))
                assertThrows(LedgerSyncDenied::class.java) { runBlocking { api.list("f", "t", "refuel", null, 100, null) } }
            }
        }
    }

    @Test fun `over-budget HTTP denials and outages never become an empty page`() {
        MockWebServer().use { server ->
            val api = LedgerSyncHttpClient(OkHttpClient(), server.url("/").toString())
            for (code in listOf(400, 401, 403, 404, 409, 429)) {
                server.enqueue(MockResponse().setResponseCode(code))
                assertThrows(LedgerSyncDenied::class.java) { runBlocking { api.list("f", "t", "refuel", null, 100, null) } }
            }
            server.enqueue(MockResponse().setResponseCode(503))
            assertThrows(IOException::class.java) { runBlocking { api.list("f", "t", "refuel", null, 100, null) } }
            server.enqueue(MockResponse().setBody("x".repeat(1024 * 1024 + 1)))
            assertThrows(IOException::class.java) { runBlocking { api.list("f", "t", "refuel", null, 100, null) } }
        }
    }

    @Test fun `cancellation interrupts a throttled body without waiting for call timeout`() = runBlocking {
        MockWebServer().use { server ->
            server.dispatcher = object : okhttp3.mockwebserver.Dispatcher() {
                override fun dispatch(request: okhttp3.mockwebserver.RecordedRequest) =
                    MockResponse().setBody(page()).throttleBody(1, 5, TimeUnit.SECONDS)
            }
            val started = java.util.concurrent.CountDownLatch(1)
            val http = OkHttpClient.Builder().eventListener(object : okhttp3.EventListener() {
                override fun responseBodyStart(call: okhttp3.Call) { started.countDown() }
            }).build()
            val api = LedgerSyncHttpClient(http, server.url("/").toString())
            val read = async(Dispatchers.IO) { api.list("f", "t", "refuel", null, 100, null) }
            withContext(Dispatchers.IO) { assertTrue(started.await(5, TimeUnit.SECONDS)) }
            withTimeout(2000) { read.cancelAndJoin() }
            assertTrue(read.isCancelled)
        }
    }
}
