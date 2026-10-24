package com.anpfuel.data.remote.charging

import com.anpfuel.domain.charging.ChargingCondition
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingObservationDraft
import com.anpfuel.domain.charging.ChargingObservationException
import com.anpfuel.domain.charging.ChargingReportReason
import com.anpfuel.domain.charging.ChargingVote
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ChargingObservationHttpClientTest {
    private val place = "11111111-1111-4111-8111-111111111111"
    private val id = "22222222-2222-4222-8222-222222222222"
    private val family = "33333333-3333-4333-8333-333333333333"
    private val draft = ChargingObservationDraft(
        placeId = place,
        connector = ChargingConnector.TYPE2,
        priceMilliBrlPerKwh = 1800,
        condition = ChargingCondition.STANDARD,
        observedDay = "2026-10-22",
        note = "public",
    )

    private fun client(server: MockWebServer) =
        ChargingObservationHttpClient(OkHttpClient(), server.url("/").toString())

    @Test fun `submit sends only allowlisted fields and exact session`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody("""{"id":"$id"}"""))
            val returned = runBlocking { client(server).submit(family, "token", id, draft) }
            assertEquals(id, returned)
            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertEquals("no-store", request.getHeader("Cache-Control"))
            val body = request.body.readUtf8()
            assertTrue(body.contains("\"family_id\":\"$family\""))
            assertTrue(body.contains("\"access_token\":\"token\""))
            assertTrue(body.contains("\"place_id\":\"$place\""))
            assertTrue(body.contains("\"price_milli_brl_per_kwh\":1800"))
            listOf("gps", "latitude", "longitude", "route", "home", "photo").forEach {
                assertTrue(!body.contains(it)) { "private field leaked: $it" }
            }
        }
    }

    @Test fun `history and owner reads parse strict typed values`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(
                MockResponse().setBody(
                    """{"items":[{"id":"$id","connector_code":"TYPE2","price_milli_brl_per_kwh":1800,"condition":"PEAK","observed_day":"2026-10-22","note":"n","helpful":2,"not_helpful":1}]}""",
                ),
            )
            server.enqueue(
                MockResponse().setBody(
                    """{"items":[{"id":"$id","place_id":"$place","connector_code":"TYPE2","condition":"STANDARD","observed_day":"2026-10-22","note":"n","revision":3}]}""",
                ),
            )
            val history = runBlocking { client(server).history(place, 20) }
            assertEquals(1, history.size)
            assertEquals(2L, history.single().helpful)
            assertEquals(ChargingCondition.PEAK, history.single().condition)
            val mine = runBlocking { client(server).mine(family, "token", 20) }
            assertEquals(3L, mine.single().revision)
            assertNull(mine.single().priceMilliBrlPerKwh)
        }
    }

    @Test fun `correction withdraw vote and report keep exact target and revision`() = runTest {
        MockWebServer().use { server ->
            val api = client(server)
            server.enqueue(MockResponse().setBody("""{"revision":4}"""))
            assertEquals(4L, runBlocking { api.correct(family, "t", id, 3, draft) })
            assertEquals("/v1/charging/observations/$id", server.takeRequest().path)
            server.enqueue(MockResponse().setBody("""{"revision":5}"""))
            assertEquals(5L, runBlocking { api.withdraw(family, "t", id, 4) })
            assertEquals("/v1/charging/observations/$id", server.takeRequest().path)
            server.enqueue(MockResponse().setBody("""{"observation_id":"$id","choice":"HELPFUL","helpful":3,"not_helpful":1}"""))
            val tally = runBlocking { api.vote(family, "t", id, ChargingVote.HELPFUL, "vote-1") }
            assertEquals(3L, tally.helpful)
            assertEquals("/v1/charging/observations/$id/helpfulness", server.takeRequest().path)
            server.enqueue(MockResponse().setBody("""{"observation_id":"$id","status":"received"}"""))
            runBlocking { api.report(family, "t", id, ChargingReportReason.SPAM, "synthetic") }
            assertEquals("/v1/charging/observations/$id/report", server.takeRequest().path)
        }
    }

    @Test fun `cancellation aborts the active request`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(
                MockResponse().setBody("""{"items":[]}""").setBodyDelay(5, TimeUnit.SECONDS),
            )
            val api = client(server)
            val result = runBlocking {
                val job = launch(Dispatchers.IO) { api.history(place, 20) }
                delay(150)
                job.cancel()
                job.join()
                job.isCancelled
            }
            assertTrue(result)
        }
    }

    @Test fun `refusals remain typed and corrupt bodies fail closed`() = runTest {
        MockWebServer().use { server ->
            val api = client(server)
            server.enqueue(MockResponse().setResponseCode(409))
            assertThrows(ChargingObservationException.Conflict::class.java) {
                runBlocking { api.correct(family, "t", id, 1, draft) }
            }
            server.enqueue(MockResponse().setResponseCode(403))
            assertThrows(ChargingObservationException.Denied::class.java) {
                runBlocking { api.vote(family, "t", id, ChargingVote.HELPFUL, "v") }
            }
            server.enqueue(MockResponse().setResponseCode(503))
            assertThrows(ChargingObservationException.Unavailable::class.java) {
                runBlocking { api.history(place, 20) }
            }
            server.enqueue(MockResponse().setBody("""{"items":[{"id":"not-a-uuid"}]}"""))
            assertThrows(ChargingObservationException.Invalid::class.java) {
                runBlocking { api.history(place, 20) }
            }
            server.enqueue(MockResponse().setBody("x".repeat(256 * 1024 + 1)))
            assertThrows(ChargingObservationException.Invalid::class.java) {
                runBlocking { api.history(place, 20) }
            }
        }
    }
}
