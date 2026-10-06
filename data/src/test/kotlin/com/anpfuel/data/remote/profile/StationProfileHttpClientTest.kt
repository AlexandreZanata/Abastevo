package com.anpfuel.data.remote.profile

import java.io.IOException
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class StationProfileHttpClientTest {
    private val id = "d6c74c23-63db-4c24-a2e5-408cb23bad26"

    @Test
    fun `anonymous read projects privacy and current representation separately`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody("""{"station_id":"$id","display_name":"Posto","revision":2,"business":{"phone":"public","cpf":"private","price":"1"},"operator":{"source":"registry"},"has_badge":true}"""))
            val profile = StationProfileHttpClient(OkHttpClient(), server.url("/").toString()).getProfile(id)!!
            assertEquals(mapOf("phone" to "public"), profile.business)
            assertTrue(profile.hasBadge)
            assertEquals(2, profile.revision)
            val request = server.takeRequest()
            assertEquals("/v1/stations/$id/profile", request.path)
            assertNull(request.getHeader("Authorization"))
            assertEquals("", request.body.readUtf8())
        }
    }

    @Test
    fun `foreign response missing profile and server refusal fail closed`() = runTest {
        MockWebServer().use { server ->
            val api = StationProfileHttpClient(OkHttpClient(), server.url("/").toString())
            server.enqueue(MockResponse().setResponseCode(404))
            assertNull(api.getProfile(id))
            server.enqueue(MockResponse().setBody("""{"station_id":"foreign","display_name":"Posto"}"""))
            assertThrows(IOException::class.java) { kotlinx.coroutines.runBlocking { api.getProfile(id) } }
            server.enqueue(MockResponse().setResponseCode(503).setBody("private upstream detail"))
            val error = assertThrows(IOException::class.java) { kotlinx.coroutines.runBlocking { api.getProfile(id) } }
            assertFalse(error.message.orEmpty().contains("private"))
        }
    }
}
