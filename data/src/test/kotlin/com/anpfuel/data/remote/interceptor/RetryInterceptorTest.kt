package com.anpfuel.data.remote.interceptor

import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class RetryInterceptorTest {

    private lateinit var server: MockWebServer

    @BeforeEach
    fun setUp() {
        server = MockWebServer()
        server.start()
    }

    @AfterEach
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun retriesUntilServerReturnsSuccess() {
        server.enqueue(MockResponse().setResponseCode(503))
        server.enqueue(MockResponse().setResponseCode(503))
        server.enqueue(MockResponse().setResponseCode(200).setBody("ok"))

        val client = OkHttpClient.Builder()
            .addInterceptor(RetryInterceptor(maxRetries = 3))
            .build()

        val response = client.newCall(
            Request.Builder().url(server.url("/listing")).build(),
        ).execute()

        response.use {
            assertEquals(200, it.code)
            assertEquals("ok", it.body?.string())
        }
        assertEquals(3, server.requestCount)
    }

    @Test
    fun doesNotRetryNonRetryableClientErrors() {
        server.enqueue(MockResponse().setResponseCode(404))

        val client = OkHttpClient.Builder()
            .addInterceptor(RetryInterceptor(maxRetries = 3))
            .build()

        val response = client.newCall(
            Request.Builder().url(server.url("/missing")).build(),
        ).execute()

        response.use {
            assertEquals(404, it.code)
        }
        assertEquals(1, server.requestCount)
    }
}
