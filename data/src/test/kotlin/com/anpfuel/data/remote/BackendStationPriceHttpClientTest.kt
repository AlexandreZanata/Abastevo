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

class BackendStationPriceHttpClientTest {

    private val stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"

    private fun client(server: MockWebServer): BackendStationPriceHttpClient =
        BackendStationPriceHttpClient(
            client = OkHttpClient.Builder()
                .connectTimeout(2L, TimeUnit.SECONDS)
                .readTimeout(2L, TimeUnit.SECONDS)
                .build(),
            baseUrl = server.url("/").toString(),
            nowMillis = { 1_000_000L },
        )

    @Test
    fun `fetches groups path with fuel filter`() {
        val server = MockWebServer()
        try {
            server.enqueue(
                MockResponse().setResponseCode(200)
                    .setBody("""{"items": [], "generated_at": "2026-09-28T00:00:00Z"}"""),
            )
            val raw = client(server).fetch(stationId, "GASOLINE_REGULAR")

            assertTrue(raw.body.contains("items"))
            assertEquals(1_000_000L, raw.fetchedAtMillis)
            val recorded = server.takeRequest()
            assertTrue(recorded.path!!.contains("/v1/stations/$stationId/prices"))
            assertTrue(recorded.path!!.contains("fuel_product=GASOLINE_REGULAR"))
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `backend down and empty body throw`() {
        val server = MockWebServer()
        try {
            server.enqueue(MockResponse().setResponseCode(500).setBody("down"))
            assertThrows(IOException::class.java) {
                client(server).fetch(stationId, null)
            }
            server.enqueue(MockResponse().setResponseCode(200).setBody("  "))
            assertThrows(IOException::class.java) {
                client(server).fetch(stationId, null)
            }
        } finally {
            server.shutdown()
        }
    }
}
