package com.anpfuel.data.remote

import java.io.IOException
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runCurrent
import java.util.concurrent.TimeUnit
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

@OptIn(kotlinx.coroutines.ExperimentalCoroutinesApi::class)
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
    fun `automatic city list carries scope and encoded cursor`() = kotlinx.coroutines.test.runTest {
        val server = MockWebServer()
        try {
            server.enqueue(MockResponse().setBody("{\"items\":[]}"))
            client(server).awaitCityList("5107925", 100, "cursor&next")
            val url = server.takeRequest().requestUrl!!
            assertEquals("5107925", url.queryParameter("municipality_code"))
            assertEquals("100", url.queryParameter("limit"))
            assertEquals("cursor&next", url.queryParameter("cursor"))
        } finally { server.shutdown() }
    }

    @Test
    fun `cancelling a directory read cancels the actual HTTP call`() = kotlinx.coroutines.test.runTest {
        val server = MockWebServer()
        val failed = java.util.concurrent.CountDownLatch(1)
        try {
            server.enqueue(MockResponse().setSocketPolicy(okhttp3.mockwebserver.SocketPolicy.NO_RESPONSE))
            val http = OkHttpClient.Builder().eventListener(object : okhttp3.EventListener() {
                override fun callFailed(call: okhttp3.Call, ioe: IOException) { failed.countDown() }
            }).build()
            val reader = DirectoryStationHttpClient(http, server.url("/").toString())
            val pending = launch { reader.awaitList(20, null) }
            runCurrent()
            org.junit.jupiter.api.Assertions.assertNotNull(server.takeRequest(3, TimeUnit.SECONDS))
            pending.cancel()
            pending.join()
            assertTrue(failed.await(3, TimeUnit.SECONDS), "cancel must close the HTTP call")
        } finally { server.shutdown() }
    }

    @Test
    fun `cancelling nearby and search cancels the actual HTTP call`() = kotlinx.coroutines.test.runTest {
        val server = MockWebServer()
        val failed = java.util.concurrent.CountDownLatch(2)
        try {
            repeat(2) { server.enqueue(MockResponse().setSocketPolicy(okhttp3.mockwebserver.SocketPolicy.NO_RESPONSE)) }
            val http = OkHttpClient.Builder().eventListener(object : okhttp3.EventListener() {
                override fun callFailed(call: okhttp3.Call, ioe: IOException) { failed.countDown() }
            }).build()
            val reader = DirectoryStationHttpClient(http, server.url("/").toString())
            val nearby = launch { reader.awaitNearby(-23.55, -46.63, 2000, 20) }
            runCurrent()
            org.junit.jupiter.api.Assertions.assertNotNull(server.takeRequest(3, TimeUnit.SECONDS))
            nearby.cancel()
            nearby.join()
            val search = launch { reader.awaitSearch("3550308", "Posto", 20) }
            runCurrent()
            org.junit.jupiter.api.Assertions.assertNotNull(server.takeRequest(3, TimeUnit.SECONDS))
            search.cancel()
            search.join()
            assertTrue(failed.await(3, TimeUnit.SECONDS), "cancel must close both HTTP calls")
        } finally { server.shutdown() }
    }

    @Test
    fun `oversized body is refused before parsing`() = kotlinx.coroutines.test.runTest {
        val server = MockWebServer()
        try {
            server.enqueue(MockResponse().setResponseCode(200).setBody("{\"items\":[" + "0".repeat(600 * 1024) + "]}"))
            assertThrows(IOException::class.java) {
                kotlinx.coroutines.runBlocking { client(server).awaitList(20, null) }
            }
        } finally { server.shutdown() }
    }

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
    fun `search carries an encoded continuation cursor`() {
        val server = MockWebServer()
        try {
            server.enqueue(MockResponse().setResponseCode(200).setBody("""{"items": []}"""))
            client(server).search("5107925", "Posto", 20, "cursor&next")
            val url = server.takeRequest().requestUrl!!
            assertEquals("cursor&next", url.queryParameter("cursor"))
            assertThrows(IllegalArgumentException::class.java) {
                client(server).search("5107925", "Posto", 20, "x".repeat(5000))
            }
            assertEquals(1, server.requestCount)
        } finally { server.shutdown() }
    }

    @Test
    fun `rejected cursor surfaces as typed http error`() = kotlinx.coroutines.test.runTest {
        val server = MockWebServer()
        try {
            server.enqueue(MockResponse().setResponseCode(400).setBody("bad cursor"))
            val error = assertThrows(DirectoryStationHttpClient.DirectoryHttpException::class.java) {
                kotlinx.coroutines.runBlocking { client(server).awaitList(20, "stale") }
            }
            assertEquals(400, error.code)
            assertTrue(error is IOException)
        } finally { server.shutdown() }
    }

    @Test
    fun `blank query lists every city station without q param`() {
        val server = MockWebServer()
        try {
            server.enqueue(MockResponse().setResponseCode(200).setBody("""{"items": []}"""))
            client(server).search("5107925", "   ", 20)
            val url = server.takeRequest().requestUrl!!
            assertEquals("5107925", url.queryParameter("municipality_code"))
            assertEquals(null, url.queryParameter("q"))
            assertThrows(IllegalArgumentException::class.java) { client(server).search("5107925", "a", 20) }
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
