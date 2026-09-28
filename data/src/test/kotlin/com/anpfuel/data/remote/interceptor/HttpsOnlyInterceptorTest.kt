package com.anpfuel.data.remote.interceptor

import okhttp3.OkHttpClient
import okhttp3.Request
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test
import java.io.IOException

class HttpsOnlyInterceptorTest {

    private val client = OkHttpClient.Builder()
        .addInterceptor(HttpsOnlyInterceptor())
        .build()

    @Test
    fun rejectsCleartextHttpRequests() {
        val error = assertThrows(IOException::class.java) {
            client.newCall(
                Request.Builder()
                    .url("http://example.com/")
                    .head()
                    .build(),
            ).execute()
        }

        assertEquals(true, error.message?.contains("Cleartext HTTP is not allowed") == true)
    }
}
