package com.anpfuel.data.remote

import com.anpfuel.application.portable.PhotoCache
import java.io.IOException
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ContributionUploadHttpClientTest {

    private class FakeCache(val bytes: ByteArray?) : PhotoCache {
        var gets = 0
        override fun put(id: String, bytes: ByteArray, capturedAtMillis: Long) = Unit
        override fun get(id: String): ByteArray? {
            gets += 1
            return bytes
        }
        override fun delete(id: String) = Unit
        override fun sweepExpired(nowMillis: Long): Int = 0
    }

    private fun client(server: MockWebServer, cache: PhotoCache): ContributionUploadHttpClient =
        ContributionUploadHttpClient(
            client = OkHttpClient.Builder()
                .connectTimeout(2L, TimeUnit.SECONDS)
                .readTimeout(2L, TimeUnit.SECONDS)
                .build(),
            baseUrl = server.url("/").toString(),
            photoCache = cache,
        )

    private fun payload(photoId: String? = null): String {
        val photo = if (photoId == null) "null" else "\"$photoId\""
        return "{\"client_submission_id\":\"cmd-1\",\"station_id\":\"d6c74c23-63db-4c24-a2e5-408cb23bad26\"," +
            "\"fuel_product\":\"GASOLINE_REGULAR\",\"amount_milli_brl\":5890,\"currency\":\"BRL\"," +
            "\"unit\":\"BRL/L\",\"condition_kind\":\"STANDARD\",\"captured_at_millis\":1000000," +
            "\"freshness\":\"fresh\",\"photo_id\":$photo,\"supersedes_observation_id\":null}"
    }

    @Test
    fun `metadata-only success returns received with idempotency headers`() = runTest {
        val server = MockWebServer()
        try {
            server.enqueue(MockResponse().setResponseCode(200).setBody("{\"status\":\"RECEIVED\"}"))
            val receipt = client(server, FakeCache(null)).submit("cmd-1", 1, payload(), "nonce-1")

            assertEquals("cmd-1", receipt.commandId)
            val recorded = server.takeRequest()
            assertTrue(recorded.path!!.contains("/v1/observations"))
            assertEquals("cmd-1", recorded.getHeader("Idempotency-Key"))
            assertEquals("nonce-1", recorded.getHeader("X-Nonce"))
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `photo reserve put complete then observation success`() = runTest {
        val server = MockWebServer()
        try {
            server.enqueue(MockResponse().setResponseCode(200).setBody("{\"upload_id\":\"up-1\"}"))
            server.enqueue(MockResponse().setResponseCode(200).setBody("{}"))
            server.enqueue(MockResponse().setResponseCode(200).setBody("{}"))
            server.enqueue(MockResponse().setResponseCode(200).setBody("{\"status\":\"VALIDATED\"}"))
            val receipt = client(server, FakeCache(ByteArray(16) { 7 }))
                .submit("cmd-1", 1, payload("photo-1"), "nonce-2")

            assertEquals(com.anpfuel.domain.repository.ContributionRemoteStatus.VALIDATED, receipt.status)
            assertEquals(4, server.requestCount)
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `expired photo falls back to metadata-only`() = runTest {
        val server = MockWebServer()
        try {
            server.enqueue(MockResponse().setResponseCode(200).setBody("{\"status\":\"RECEIVED\"}"))
            val cache = FakeCache(null)
            client(server, cache).submit("cmd-1", 1, payload("photo-gone"), "nonce-3")

            assertEquals(1, server.requestCount)
            assertEquals(1, cache.gets)
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `object success with finalize failure throws`() = runTest {
        val server = MockWebServer()
        try {
            server.enqueue(MockResponse().setResponseCode(200).setBody("{\"upload_id\":\"up-1\"}"))
            server.enqueue(MockResponse().setResponseCode(200).setBody("{}"))
            server.enqueue(MockResponse().setResponseCode(500).setBody("down"))

            assertThrows(IOException::class.java) {
                kotlinx.coroutines.runBlocking {
                    client(server, FakeCache(ByteArray(16) { 7 }))
                        .submit("cmd-1", 1, payload("photo-1"), "nonce-4")
                }
            }
        } finally {
            server.shutdown()
        }
    }
}
