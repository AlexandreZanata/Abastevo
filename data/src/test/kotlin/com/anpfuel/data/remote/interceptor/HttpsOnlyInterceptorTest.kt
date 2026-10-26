package com.anpfuel.data.remote.interceptor

import java.io.IOException
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class HttpsOnlyInterceptorTest {
    private val server = MockWebServer()

    @AfterEach fun stop() { server.shutdown() }

    @Test
    fun `default client rejects cleartext`() {
        server.enqueue(MockResponse().setBody("{}"))
        server.start()
        val client = OkHttpClient.Builder().addInterceptor(HttpsOnlyInterceptor(allowLocalProof = false)).build()
        assertThrows(IOException::class.java) {
            client.newCall(Request.Builder().url(server.url("/v1/stations")).build()).execute().close()
        }
    }

    @Test
    fun `local proof exception stays on the loopback host`() {
        server.enqueue(MockResponse().setBody("{}"))
        server.start()
        val client = OkHttpClient.Builder().addInterceptor(HttpsOnlyInterceptor(allowLocalProof = true)).build()
        // MockWebServer uses another loopback port, so it must still be refused.
        assertThrows(IOException::class.java) {
            client.newCall(Request.Builder().url(server.url("/v1/stations")).build()).execute().close()
        }
    }
}
