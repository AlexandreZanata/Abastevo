package com.anpfuel.data.remote.ranking

import java.util.concurrent.TimeUnit
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

class PriceRankingHttpClientTest {

    private fun station(id: String = "11111111-1111-4111-8111-111111111111", amount: Long = 5990) =
        """{"station_id":"$id","amount_milli_brl":$amount,"currency":"BRL","collected_on":"2026-10-18","state":"DF","ibge_code":"5300108"}"""

    private fun page(next: String = "null") = """{"interpretation":{"source":{"kind":"ANP_WEEKLY","survey_week_start":"2026-10-12"},"product":{"fuel":"GASOLINE_REGULAR","unit":"L"},"condition":{"kind":"STANDARD"},"geography":{"level":"COUNTRY","country":"BR"},"metric":{"kind":"OBSERVATION_PRICE","group_by":"STATION","sort":"DESC"},"policy_version":"mex-exploration-v1"},"source":{"kind":"ANP_WEEKLY","survey_week_start":"2026-10-12","survey_week_end":"2026-10-18","revision_id":"eeeeeeee-6666-4666-8666-000000000001","published_at":"2026-10-19T00:00:00Z","source_url":"https://example.test/x","source_checksum":"abc"},"coverage":{"source_observations":10,"stations_with_price":8,"source_stations":9,"municipalities":3,"states":2,"unlocated_observations":0,"sampled":true,"national_completeness_claimed":false},"stations":[${station()}],"areas":[],"next_cursor":$next,"generated_at":"2026-10-19T00:00:00Z"}"""

    private fun params(cursor: String? = null) = PriceRankingParams(
        fuel = "GASOLINE_REGULAR", unit = "L", condition = "STANDARD",
        geography = "COUNTRY", uf = null, ibgeCode = null,
        metric = "OBSERVATION_PRICE", groupBy = "STATION", sort = "DESC",
        revision = "eeeeeeee-6666-4666-8666-000000000001", limit = 20, cursor = cursor,
    )

    @Test fun `typed pages preserve server order and bind revision`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody(page()))
            val api = PriceRankingHttpClient(OkHttpClient(), server.url("/").toString())
            val result = runBlocking { api.rank(params()) }
            assertEquals(1, result.stations.size)
            assertEquals(5990L, result.stations.single().amountMilliBrl)
            assertEquals("eeeeeeee-6666-4666-8666-000000000001", result.revisionId)
            assertTrue(result.generatedAt.isNotEmpty())
            val request = server.takeRequest(1, TimeUnit.SECONDS)
            assertEquals("GASOLINE_REGULAR", request?.requestUrl?.queryParameter("fuel"))
            assertEquals("COUNTRY", request?.requestUrl?.queryParameter("geography"))
            assertTrue(request?.requestUrl?.queryParameter("survey_week_start") == null)
            assertNull(request?.getHeader("Authorization"))
        }
    }

    @Test fun `corrupt oversize and stale cursors fail closed`() = runTest {
        MockWebServer().use { server ->
            val api = PriceRankingHttpClient(OkHttpClient(), server.url("/").toString())
            server.enqueue(MockResponse().setBody("x".repeat(1024 * 1024 + 1)))
            assertThrows(PriceRankingUnavailable::class.java) { runBlocking { api.rank(params()) } }
            server.enqueue(MockResponse().setBody(page().replace("amount_milli_brl", "amount")))
            assertThrows(PriceRankingUnavailable::class.java) { runBlocking { api.rank(params()) } }
            server.enqueue(MockResponse().setBody(page(next = "\"repeat\"")))
            assertThrows(PriceRankingUnavailable::class.java) { runBlocking { api.rank(params("repeat")) } }
            server.enqueue(MockResponse().setResponseCode(400).setBody("""{"error":"bad-filter"}"""))
            assertThrows(PriceRankingUnavailable::class.java) { runBlocking { api.rank(params()) } }
        }
    }

    @Test fun `cancellation stops transport`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody(page()).throttleBody(1, 5, TimeUnit.SECONDS))
            val api = PriceRankingHttpClient(OkHttpClient(), server.url("/").toString())
            val job = launch { api.rank(params()) }
            job.cancel()
            job.join()
            assertTrue(job.isCancelled)
        }
    }

    @Test fun `municipal queries require exact ibge codes`() = runTest {
        MockWebServer().use { server ->
            val api = PriceRankingHttpClient(OkHttpClient(), server.url("/").toString())
            assertThrows(PriceRankingUnavailable::class.java) {
                runBlocking {
                    api.rank(params().copy(geography = "MUNICIPALITY", ibgeCode = "short"))
                }
            }
            assertEquals(0, server.requestCount)
        }
    }
}
