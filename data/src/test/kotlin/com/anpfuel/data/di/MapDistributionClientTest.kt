package com.anpfuel.data.di

import com.anpfuel.data.BuildConfig
import com.anpfuel.data.map.MapDistributionConfig
import java.io.IOException
import java.util.concurrent.TimeUnit
import okhttp3.Request
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class MapDistributionClientTest {
    private fun config(url: String) = MapDistributionConfig(url, "proof", emptyMap(), false)
    @Test fun `HTTP is limited to the explicit debug loopback origin`() {
        MockWebServer().use { local ->
            local.start()
            local.enqueue(MockResponse().setBody("proof"))
            val client = MapModule.mapDistributionHttpClient(config(local.url("/").toString()))
            val request = Request.Builder().url(local.url("/manifest")).build()
            if (BuildConfig.DEBUG) {
                client.newCall(request).execute().use { assertEquals(200, it.code) }
            } else {
                assertThrows(IOException::class.java) { client.newCall(request).execute() }
            }
        }
    }
    @Test fun `a redirect cannot expand the debug HTTP exception to another port`() {
        MockWebServer().use { first -> MockWebServer().use { other ->
            first.start(); other.start()
            first.enqueue(MockResponse().setResponseCode(302).setHeader("Location", other.url("/private")))
            other.enqueue(MockResponse().setBody("must not be read"))
            val client = MapModule.mapDistributionHttpClient(config(first.url("/").toString()))
            assertThrows(IOException::class.java) {
                client.newCall(Request.Builder().url(first.url("/manifest")).build()).execute()
            }
            assertNull(other.takeRequest(100, TimeUnit.MILLISECONDS))
        } }
    }
    @Test fun `non-loopback configuration retains HTTPS enforcement even in debug`() {
        val client = MapModule.mapDistributionHttpClient(config("http://192.0.2.1:18765"))
        assertThrows(IOException::class.java) {
            client.newCall(Request.Builder().url("http://127.0.0.1:1/manifest").build()).execute()
        }
    }
}
