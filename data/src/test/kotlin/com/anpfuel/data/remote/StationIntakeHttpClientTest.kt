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
import org.json.JSONObject

class StationIntakeHttpClientTest {

    private fun client(server: MockWebServer): StationIntakeHttpClient =
        StationIntakeHttpClient(
            client = OkHttpClient.Builder()
                .connectTimeout(2L, TimeUnit.SECONDS)
                .readTimeout(2L, TimeUnit.SECONDS)
                .build(),
            baseUrl = server.url("/").toString(),
        )

    @Test
    fun `submit posts session plus proposal without double v1`() {
        val server = MockWebServer()
        try {
            server.enqueue(
                MockResponse().setResponseCode(201)
                    .setBody("""{"id": "sug-1", "state": "pending"}"""),
            )
            val raw = client(server).submit(
                "fam-1", "tok-1", "key-1",
                """{"display_name": "Posto Novo"}""",
            )

            assertTrue(raw.contains("sug-1"))
            val recorded = server.takeRequest()
            assertEquals("/v1/stations/suggestions", recorded.path)
            assertTrue(!recorded.path!!.contains("/v1/v1"))
            assertTrue(recorded.getHeader("Cache-Control") == "no-store")
            val body = recorded.body.readUtf8()
            assertTrue(body.contains("access_token"))
            assertTrue(body.contains("client_submission_id"))
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `correction sends a private proposal and refuses a mismatched receipt`() {
        MockWebServer().use { server ->
            val station = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
            val key = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
            server.enqueue(MockResponse().setResponseCode(201).setBody(
                """{"id":"correction-1","station_id":"$station","client_submission_id":"$key"}""",
            ))
            val client = client(server)
            val value = JSONObject().put("status", "permanently_closed")
            client.submitCorrection("family", "token", station, key, "operating_status", value)
            val request = server.takeRequest()
            assertEquals("/v1/stations/$station/corrections", request.path)
            assertEquals("no-store", request.getHeader("Cache-Control"))
            val body = JSONObject(request.body.readUtf8())
            assertEquals("token", body.getString("access_token"))
            assertEquals(key, body.getString("client_submission_id"))
            assertEquals("operating_status", body.getString("field"))
            assertEquals("permanently_closed", body.getJSONObject("proposed_value").getString("status"))

            server.enqueue(MockResponse().setResponseCode(201).setBody("""{"id":"unbound"}"""))
            assertThrows(IOException::class.java) {
                client.submitCorrection("family", "token", station, key, "operating_status", value)
            }
        }
    }

    @Test
    fun `mine status and cancel hit owner routes`() {
        val server = MockWebServer()
        try {
            server.enqueue(MockResponse().setResponseCode(200).setBody("""{"items": []}"""))
            client(server).mine("fam-1", "tok-1")
            assertEquals("/v1/stations/suggestions/mine", server.takeRequest().path)

            server.enqueue(MockResponse().setResponseCode(200).setBody("""{"id": "s", "state": "pending"}"""))
            client(server).status("fam-1", "tok-1", "s")
            assertEquals("/v1/stations/suggestions/s/status", server.takeRequest().path)

            server.enqueue(MockResponse().setResponseCode(200).setBody("""{"status": "cancelled"}"""))
            client(server).cancel("fam-1", "tok-1", "s")
            assertEquals("/v1/stations/suggestions/s/cancel", server.takeRequest().path)
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `non-2xx empty and malformed proposal throw`() {
        val server = MockWebServer()
        try {
            server.enqueue(MockResponse().setResponseCode(401).setBody("denied"))
            assertThrows(IOException::class.java) {
                client(server).mine("fam-1", "tok-1")
            }
            server.enqueue(MockResponse().setResponseCode(200).setBody("  "))
            assertThrows(IOException::class.java) {
                client(server).status("fam-1", "tok-1", "s")
            }
            assertThrows(IOException::class.java) {
                client(server).submit("fam-1", "tok-1", "key-1", "{broken")
            }
        } finally {
            server.shutdown()
        }
    }
}
