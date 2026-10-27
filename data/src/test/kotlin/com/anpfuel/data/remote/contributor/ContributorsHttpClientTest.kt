package com.anpfuel.data.remote.contributor

import com.anpfuel.domain.contributor.ContributorException
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ContributorsHttpClientTest {

    private val page = """
        {"items":[{"display_name":"Ana",
          "links":[{"platform":"github","handle":"octocat","url":"https://github.com/octocat"}],
          "github_login":"octocat","contributor_since":"2026-03","roles":[]}],
         "next_cursor":"cursor-2"}
    """.trimIndent()

    private fun client(server: MockWebServer) =
        ContributorsHttpClient(OkHttpClient(), server.url("/").toString())

    @Test fun `list sends no session and parses the page`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody(page))
            val result = runBlocking { client(server).page(null) }
            assertEquals(1, result.items.size)
            assertEquals("cursor-2", result.nextCursor)
            assertEquals("Ana", result.items[0].displayName)
            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertEquals("GET", request.method)
            assertEquals("/v1/contributors", request.path)
            assertEquals("application/json", request.getHeader("Accept"))
            val body = request.body.readUtf8()
            assertTrue(!body.contains("family_id") && !body.contains("access_token"))
        }
    }

    @Test fun `cursor travels as query parameter`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody(page))
            runBlocking { client(server).page("cursor-1") }
            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertEquals("/v1/contributors?cursor=cursor-1", request.path)
        }
    }

    @Test fun `denied stays explicit`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setResponseCode(403))
            assertThrows(ContributorException.Denied::class.java) {
                runBlocking { client(server).page(null) }
            }
        }
    }

    @Test fun `server down surfaces offline`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setResponseCode(503))
            assertThrows(ContributorException.Unavailable::class.java) {
                runBlocking { client(server).page(null) }
            }
        }
    }

    @Test fun `unreachable host surfaces offline`() = runTest {
        val closed = ContributorsHttpClient(OkHttpClient(), "http://127.0.0.1:9/")
        assertThrows(ContributorException.Offline::class.java) {
            runBlocking { closed.page(null) }
        }
    }
}
