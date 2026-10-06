package com.anpfuel.data.remote

import com.anpfuel.domain.community.FeedCity
import com.anpfuel.domain.community.FeedQuery
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.launch
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.ExperimentalCoroutinesApi
import okhttp3.EventListener
import okhttp3.Call
import okhttp3.mockwebserver.SocketPolicy
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.TimeUnit
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class CityCommunityFeedHttpClientTest {
    private val query = FeedQuery(FeedCity("5107925", BrazilianState.MATO_GROSSO, "Sorriso"), FuelProduct.GASOLINE_PREMIUM)
    private val payload = """{"state":"MT","municipality_code":"5107925","fuel_product":"GASOLINE_ADDITIVED","sort":"recent","generated_at":"2026-10-06T12:00:00Z","next_cursor":null,"items":[{"station_id":"00000000-0000-0000-0000-000000000001","station_name":"Synthetic station","fuel_product":"GASOLINE_ADDITIVED","unit":"L","amount_milli_brl":5999,"currency":"BRL","source":"COMMUNITY","condition":"STANDARD","confidence":"LOW","updated_at":"2026-10-06T11:00:00Z","expires_at":"2026-10-07T11:00:00Z","supporters":1,"confirmations":0,"version":1}]}"""
    @Test fun `exact money and named legacy fuel bridge are preserved`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody(payload))
            val page = CityCommunityFeedHttpClient(OkHttpClient(), server.url("/").toString()).read(query)
            assertEquals(5999L, page.items.single().amountMilliBrl)
            assertEquals(FuelProduct.GASOLINE_PREMIUM, page.items.single().fuel)
            val request = server.takeRequest()
            assertEquals("5107925", request.requestUrl!!.queryParameter("municipality_code"))
            assertEquals("GASOLINE_ADDITIVED", request.requestUrl!!.queryParameter("fuel_product"))
            assertNull(request.getHeader("Authorization"))
        }
    }
    @Test fun `cross city official nonstandard fractional and mismatched unit responses fail closed`() {
        for ((old, replacement) in listOf("5107925" to "5103403", "COMMUNITY" to "ANP", "STANDARD" to "CASH", "5999" to "5.999", "\"unit\":\"L\"" to "\"unit\":\"M3\"", "GASOLINE_ADDITIVED" to "GASOLINE_PREMIUM")) {
            assertThrows(Exception::class.java) { CityCommunityFeedHttpClient.decode(payload.replace(old, replacement), query) }
        }
    }
    @Test fun `backend outage is not an empty successful feed`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setResponseCode(503).setBody("private information"))
            val result = runCatching { CityCommunityFeedHttpClient(OkHttpClient(), server.url("/").toString()).read(query) }
            assertTrue(result.isFailure)
            assertFalse(result.exceptionOrNull()!!.message!!.contains("private information"))
        }
    }
    @Test fun `leaving feed cancels the actual HTTP call`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setSocketPolicy(SocketPolicy.NO_RESPONSE))
            val cancelled = AtomicBoolean(false)
            val client = OkHttpClient.Builder().eventListener(object : EventListener() {
                override fun canceled(call: Call) { cancelled.set(true) }
            }).build()
            val job = launch { CityCommunityFeedHttpClient(client, server.url("/").toString()).read(query) }
            runCurrent()
            assertNotNull(server.takeRequest(5, TimeUnit.SECONDS))
            job.cancelAndJoin()
            assertTrue(cancelled.get())
        }
    }

    @Test fun `malformed deeply nested and oversized bodies fail without exhausting memory`() {
        val deep = payload.dropLast(1) + ",\"extra\":" + "[".repeat(50) + "0" + "]".repeat(50) + "}"
        assertThrows(IllegalArgumentException::class.java) { CityCommunityFeedHttpClient.decode(deep, query) }
        assertThrows(IllegalArgumentException::class.java) { CityCommunityFeedHttpClient.decode(" ".repeat(256 * 1024 + 1), query) }
        assertEquals("Station {A}", CityCommunityFeedHttpClient.decode(payload.replace("Synthetic station", "Station {A}"), query).items.single().stationName)
    }

}
