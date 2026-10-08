package com.anpfuel.data.remote

import java.io.IOException
import java.util.concurrent.TimeUnit
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class DirectoryStationHttpClientTest {

    private fun client(server: MockWebServer): DirectoryStationHttpClient =
        DirectoryStationHttpClient(
            client = OkHttpClient.Builder()
                .connectTimeout(2L, TimeUnit.SECONDS)
                .readTimeout(2L, TimeUnit.SECONDS)
                .build(),
            baseUrl = server.url("/").toString(),
        )

    @Test
    fun `city scoped station name search encodes input and rejects malformed bounds`() {
        val server = MockWebServer()
        try {
            server.enqueue(MockResponse().setResponseCode(200).setBody("""{"items": []}"""))
            client(server).search("5103403", "Posto & Centro", 20)
            val url = server.takeRequest().requestUrl!!
            assertEquals("5103403", url.queryParameter("municipality_code"))
            assertEquals("Posto & Centro", url.queryParameter("q"))
            assertThrows(IllegalArgumentException::class.java) { client(server).search("invalid", "Posto", 20) }
            assertThrows(IllegalArgumentException::class.java) { client(server).search("5103403", "x", 20) }
            assertThrows(IllegalArgumentException::class.java) { client(server).search("5103403", "Posto", 101) }
            assertEquals(1, server.requestCount)
        } finally { server.shutdown() }
    }

    @Test
    fun `lists stations without double v1`() {
        val server = MockWebServer()
        try {
            server.enqueue(
                MockResponse().setResponseCode(200)
                    .setBody("""{"items": [], "generated_at": "2026-09-28T00:00:00Z"}"""),
            )
            val raw = client(server).list(limit = 20, cursor = null)

            assertTrue(raw.contains("items"))
            val recorded = server.takeRequest()
            assertTrue(recorded.path!!.startsWith("/v1/stations?"))
            assertTrue(!recorded.path!!.contains("/v1/v1"))
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `fetches nearby with bounded params and detail by uuid`() {
        val server = MockWebServer()
        val id = "d6c74c23-63db-4c24-a2e5-408cb23bad26"
        try {
            server.enqueue(MockResponse().setResponseCode(200).setBody("""{"items": []}"""))
            client(server).nearby(lat = -23.55, lon = -46.63, radiusMeters = 2000, limit = 5)
            val recorded = server.takeRequest()
            assertTrue(recorded.path!!.contains("/v1/stations/nearby"))
            assertTrue(recorded.path!!.contains("lat=-23.55"))

            server.enqueue(MockResponse().setResponseCode(200).setBody("""{}"""))
            client(server).detail(id)
            val detailRecorded = server.takeRequest()
            assertEquals("/v1/stations/$id", detailRecorded.path)
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `rejects out of range nearby params before network`() {
        val server = MockWebServer()
        try {
            assertThrows(IllegalArgumentException::class.java) {
                client(server).nearby(lat = 91.0, lon = -46.63, radiusMeters = 2000, limit = 5)
            }
            assertEquals(0, server.requestCount)
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `non-2xx and empty body throw`() {
        val server = MockWebServer()
        try {
            server.enqueue(MockResponse().setResponseCode(404).setBody("not found"))
            assertThrows(IOException::class.java) {
                client(server).detail("d6c74c23-63db-4c24-a2e5-408cb23bad26")
            }
            server.enqueue(MockResponse().setResponseCode(200).setBody("  "))
            assertThrows(IOException::class.java) {
                client(server).list(limit = 1, cursor = null)
            }
        } finally {
            server.shutdown()
        }
    }
    @Test
    fun `exact cnpj lookup distinguishes not found from outage`() {
        val server = MockWebServer()
        try {
            server.enqueue(MockResponse().setResponseCode(200).setBody("{}"))
            assertEquals("{}", client(server).byCnpj("04218406000104"))
            assertEquals("/v1/stations/by-cnpj/04218406000104", server.takeRequest().path)
            server.enqueue(MockResponse().setResponseCode(404))
            org.junit.jupiter.api.Assertions.assertNull(client(server).byCnpj("11222333000181"))
            server.enqueue(MockResponse().setResponseCode(503))
            assertThrows(IOException::class.java) { client(server).byCnpj("04218406000104") }
            assertThrows(IllegalArgumentException::class.java) { client(server).byCnpj("../auth") }
            assertEquals(3, server.requestCount)
        } finally { server.shutdown() }
    }
}
