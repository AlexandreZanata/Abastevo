package com.anpfuel.data.remote

import com.anpfuel.data.remote.interceptor.RetryInterceptor
import okhttp3.Request
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.IOException
import java.util.concurrent.TimeUnit

class OkHttpClientFactoryTest {

    @Test
    fun createsClientWithExpectedTimeoutsAndRetryDefaults() {
        val client = OkHttpClientFactory.create()

        assertEquals(TimeUnit.SECONDS.toMillis(30), client.connectTimeoutMillis.toLong())
        assertEquals(TimeUnit.SECONDS.toMillis(60), client.readTimeoutMillis.toLong())
        assertEquals(TimeUnit.SECONDS.toMillis(60), client.writeTimeoutMillis.toLong())
        assertTrue(client.interceptors.size >= 3)
    }

    @Test
    fun factoryClientRejectsCleartextHttp() {
        val client = OkHttpClientFactory.create()

        val error = assertThrows(IOException::class.java) {
            client.newCall(
                Request.Builder()
                    .url("http://example.com/")
                    .head()
                    .build(),
            ).execute()
        }

        assertTrue(error.message?.contains("Cleartext HTTP is not allowed") == true)
    }

    @Test
    fun defaultRetryInterceptorAllowsThreeRetries() {
        assertEquals(3, RetryInterceptor.DEFAULT_MAX_RETRIES)
    }
}
