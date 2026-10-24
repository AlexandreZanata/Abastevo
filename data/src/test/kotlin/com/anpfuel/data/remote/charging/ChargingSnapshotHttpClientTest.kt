package com.anpfuel.data.remote.charging

import java.io.File
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class ChargingSnapshotHttpClientTest {

    private val placeId = "11111111-1111-4111-8111-111111111111"
    private val groupId = "22222222-2222-4222-8222-222222222222"
    private val key = "c".repeat(64)
    private val group = """{"id":"$groupId","entity_key":"$key","external_id":"OCM-7","connector_code":"CCS2","raw_connector_code":"CCS2","current":"DC","power_w":150000,"reported_quantity":2}"""
    private val item = """{"id":"$placeId","entity_key":"$key","external_id":"OCM-7","name":"Eletroposto DF-7","country":"BR","latitude":"-15.7939","longitude":"-47.8828","public_access":"PUBLIC","reported_points":2}"""
    private fun page(next: String = "null") =
        """{"items":[$item],"next_cursor":$next,"generated_at":"2026-10-22T00:00:00Z"}"""
    private fun detail() = """${item.dropLast(1)},"groups":[$group],"generated_at":"2026-10-22T00:00:00Z"}"""
    private fun coverage() =
        """{"profile":"ocm-br-20261021-v1","source_revision":"rev-1","places":1298,"groups":3387,"vehicles":181,"generated_at":"2026-10-22T00:00:00Z"}"""

    @Test fun `guest reads carry no credentials and raw enums are retained`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody(page()))
            val api = ChargingSnapshotHttpClient(OkHttpClient(), server.url("/").toString())
            val result = api.places(profile = "ocm-br-20261021-v1", limit = 20, cursor = null)
            assertEquals("Eletroposto DF-7", result.items.single().name)
            val request = server.takeRequest(1, TimeUnit.SECONDS)
            assertNull(request?.getHeader("Authorization"))
            assertEquals("ocm-br-20261021-v1", request?.requestUrl?.queryParameter("profile"))
            assertEquals("20", request?.requestUrl?.queryParameter("limit"))
            assertEquals("no-store", request?.getHeader("Cache-Control"))
            server.enqueue(MockResponse().setBody(detail().replace("CCS2", "MEGAWATT_X")))
            val future = api.placeDetail(placeId)
            assertEquals("MEGAWATT_X", future.groups.single().rawConnectorCode)
        }
    }

    @Test fun `no-store is honored and cursor repeats fail closed`() = runTest {
        MockWebServer().use { server ->
            val api = ChargingSnapshotHttpClient(OkHttpClient(), server.url("/").toString())
            server.enqueue(MockResponse().setBody(page()))
            runBlocking { api.places(profile = "ocm-br-20261021-v1", limit = 20, cursor = null) }
            server.enqueue(MockResponse().setBody(page()))
            runBlocking { api.places(profile = "ocm-br-20261021-v1", limit = 20, cursor = null) }
            assertEquals(2, server.requestCount)
            server.enqueue(MockResponse().setBody(page(next = "\"repeat\"")))
            assertThrows(ChargingSnapshotUnavailable::class.java) {
                runBlocking { api.places(profile = "ocm-br-20261021-v1", limit = 20, cursor = "repeat") }
            }
            server.enqueue(MockResponse().setResponseCode(400).setBody("""{"error":"bad-filter"}"""))
            assertThrows(ChargingSnapshotUnavailable::class.java) {
                runBlocking { api.places(profile = "licensed-operator-v1", limit = 20, cursor = null) }
            }
        }
    }

    @Test fun `corrupt oversize and private bodies never reach callers`() = runTest {
        MockWebServer().use { server ->
            val api = ChargingSnapshotHttpClient(OkHttpClient(), server.url("/").toString())
            server.enqueue(MockResponse().setBody("x".repeat(1024 * 1024 + 1)))
            assertThrows(ChargingSnapshotUnavailable::class.java) { runBlocking { api.places(profile = "p", limit = 20, cursor = null) } }
            server.enqueue(MockResponse().setBody(page().replace(placeId, "not-a-uuid")))
            assertThrows(ChargingSnapshotUnavailable::class.java) { runBlocking { api.places(profile = "p", limit = 20, cursor = null) } }
            server.enqueue(MockResponse().setBody(page(item.dropLast(1) + ""","reporter":"user-9"}""")))
            assertThrows(ChargingSnapshotUnavailable::class.java) { runBlocking { api.places(profile = "p", limit = 20, cursor = null) } }
            server.enqueue(MockResponse().setBody(coverage()))
            val got = runBlocking { api.coverage(profile = "ocm-br-20261021-v1") }
            assertEquals(1298L, got.places)
            assertEquals("rev-1", got.sourceRevision)
        }
    }

    @Test fun `cancellation stops transport without caching`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody(page()).throttleBody(1, 5, TimeUnit.SECONDS))
            val api = ChargingSnapshotHttpClient(OkHttpClient(), server.url("/").toString())
            val job = launch { api.places(profile = "ocm-br-20261021-v1", limit = 20, cursor = null) }
            job.cancel()
            job.join()
            assertTrue(job.isCancelled)
        }
    }

    @Test fun `bad filters and identifiers fail before the wire`() {
        MockWebServer().use { server ->
            val api = ChargingSnapshotHttpClient(OkHttpClient(), server.url("/").toString())
            assertThrows(ChargingSnapshotUnavailable::class.java) {
                runBlocking { api.places(profile = "", limit = 20, cursor = null) }
            }
            assertThrows(ChargingSnapshotUnavailable::class.java) {
                runBlocking { api.places(profile = "p", limit = 101, cursor = null) }
            }
            assertThrows(ChargingSnapshotUnavailable::class.java) { runBlocking { api.placeDetail("nope") } }
            assertEquals(0, server.requestCount)
        }
    }
}
