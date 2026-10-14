package com.anpfuel.data.remote

import com.anpfuel.domain.community.FeedCity
import com.anpfuel.domain.valueobject.BrazilianState
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.json.JSONArray
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class CityCommunitySummaryHttpClientTest {
    private val city=FeedCity("5103403",BrazilianState.MATO_GROSSO,"Synthetic city")
    private fun payload()=JSONObject().put("state","MT").put("municipality_code",city.code).put("refresh_after_seconds",15)
        .put("generated_at","2026-10-14T12:00:00Z").put("items",JSONArray().put(JSONObject()
            .put("fuel_product","GASOLINE_REGULAR").put("unit","L").put("currency","BRL").put("source","COMMUNITY")
            .put("condition","STANDARD").put("average_milli_brl",5032).put("minimum_milli_brl",5000).put("maximum_milli_brl",5064)
            .put("station_count",65).put("updated_at","2026-10-14T11:00:00Z").put("expires_at","2026-10-14T13:00:00Z")))
    @Test fun `anonymous summary is a city request without pagination GPS or identity`()=runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody(payload().toString()));server.start()
            val summary=CityCommunityFeedHttpClient(OkHttpClient(),server.url("/").toString()).readSummary(city)
            assertEquals(65,summary.items.single().stationCount);assertEquals(5032L,summary.items.single().averageMilliBrl)
            val request=server.takeRequest();assertEquals("/v1/community/summary?state=MT&municipality_code=5103403",request.path)
            assertNull(request.getHeader("Authorization"));assertEquals(0,request.bodySize)
        }
    }
    @Test fun `summary refuses wrong city source units counts fractional money bounds duplicate products and expired samples`() {
        val invalid=listOf<(JSONObject)->Unit>(
            {it.put("state","SP")},{it.put("municipality_code","5107925")},
            {it.getJSONArray("items").getJSONObject(0).put("source","ANP")},
            {it.getJSONArray("items").getJSONObject(0).put("currency","USD")},
            {it.getJSONArray("items").getJSONObject(0).put("unit","M3")},
            {it.getJSONArray("items").getJSONObject(0).put("station_count",0)},
            {it.getJSONArray("items").getJSONObject(0).put("average_milli_brl",5.032)},
            {it.getJSONArray("items").getJSONObject(0).put("average_milli_brl",4999)},
            {it.getJSONArray("items").getJSONObject(0).put("minimum_milli_brl",0)},
            {it.getJSONArray("items").getJSONObject(0).put("maximum_milli_brl",1000001)},
            {it.getJSONArray("items").getJSONObject(0).put("expires_at","2026-10-14T12:00:00Z")},
            {it.getJSONArray("items").put(it.getJSONArray("items").getJSONObject(0))},
        )
        invalid.forEach { mutate ->val doc=payload();mutate(doc);assertThrows(IllegalArgumentException::class.java){CityCommunityFeedHttpClient.decodeSummary(doc.toString(),city)} }
        val empty=payload().put("items",JSONArray());assertTrue(CityCommunityFeedHttpClient.decodeSummary(empty.toString(),city).items.isEmpty())
        assertThrows(IllegalArgumentException::class.java){CityCommunityFeedHttpClient.decodeSummary("[".repeat(13)+"]".repeat(13),city)}
        assertThrows(IllegalArgumentException::class.java){CityCommunityFeedHttpClient.decodeSummary(" ".repeat(262145),city)}
    }
}
