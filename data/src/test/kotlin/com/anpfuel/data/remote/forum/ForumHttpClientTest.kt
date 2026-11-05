package com.anpfuel.data.remote.forum

import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumPostKind
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.util.concurrent.TimeUnit

class ForumHttpClientTest {

    private val categoriesJson = """
        {
            "items": [
                {
                    "code": "FUELS",
                    "name": "Combustíveis",
                    "description": "Preços e tributação"
                }
            ],
            "taxonomy_version": 1
        }
    """.trimIndent()

    private val feedJson = """
        {
            "items": [
                {
                    "id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
                    "kind": "EDITORIAL",
                    "category": "FUELS",
                    "title": "Entenda a formação de preços",
                    "excerpt": "Guia explicativo.",
                    "first_published_at": "2026-11-05T12:00:00Z",
                    "updated_at": "2026-11-05T12:00:00Z",
                    "revision": 1,
                    "comments_enabled": true
                }
            ],
            "next_cursor": "cursor-token-123",
            "snapshot": "rev-1",
            "fetched_at": "2026-11-05T12:00:00Z"
        }
    """.trimIndent()

    private val detailJson = """
        {
            "id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
            "kind": "EDITORIAL",
            "category": "FUELS",
            "title": "Entenda a formação de preços",
            "excerpt": "Guia explicativo.",
            "body_text": "Corpo completo detalhado.",
            "first_published_at": "2026-11-05T12:00:00Z",
            "updated_at": "2026-11-05T12:00:00Z",
            "revision": 1,
            "comments_enabled": true
        }
    """.trimIndent()

    private fun client(server: MockWebServer) =
        ForumHttpClient(OkHttpClient(), server.url("/").toString())

    @Test
    fun `getCategories sends GET request and parses categories`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setResponseCode(200).setBody(categoriesJson))
            val result = client(server).getCategories()
            assertEquals(1, result.items.size)
            assertEquals(ForumCategory.FUELS, result.items[0].code)

            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertEquals("GET", request.method)
            assertEquals("/v1/forum/categories", request.path)
            assertEquals("application/json", request.getHeader("Accept"))
        }
    }

    @Test
    fun `getFeed formats query parameters and parses feed`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setResponseCode(200).setBody(feedJson))
            val result = client(server).getFeed(
                category = ForumCategory.FUELS,
                kind = ForumPostKind.EDITORIAL,
                limit = 10,
                cursor = "test-cursor",
            )
            assertEquals(1, result.items.size)
            assertEquals("cursor-token-123", result.nextCursor)

            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertEquals("GET", request.method)
            assertTrue(request.path!!.startsWith("/v1/forum/posts?"))
            assertTrue(request.path!!.contains("limit=10"))
            assertTrue(request.path!!.contains("category=FUELS"))
            assertTrue(request.path!!.contains("kind=EDITORIAL"))
            assertTrue(request.path!!.contains("cursor=test-cursor"))
        }
    }

    @Test
    fun `getPostDetail sends GET by ID and parses detail`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setResponseCode(200).setBody(detailJson))
            val postId = "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d"
            val detail = client(server).getPostDetail(postId)
            assertEquals(postId, detail.id)
            assertEquals("Corpo completo detalhado.", detail.bodyText)

            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertEquals("GET", request.method)
            assertEquals("/v1/forum/posts/$postId", request.path)
        }
    }

    @Test
    fun `handles 404 forum disabled response`() = runTest {
        MockWebServer().use { server ->
            val errBody = """{"error": {"code": "forum.disabled", "message": "forum is currently disabled"}}"""
            server.enqueue(MockResponse().setResponseCode(404).setBody(errBody))

            val ex = assertThrows(ForumException.Disabled::class.java) {
                runBlocking { client(server).getCategories() }
            }
            assertEquals("forum.disabled", ex.code)
        }
    }

    @Test
    fun `handles 404 post not found response`() = runTest {
        MockWebServer().use { server ->
            val errBody = """{"error": {"code": "forum.not-found", "message": "post not found"}}"""
            server.enqueue(MockResponse().setResponseCode(404).setBody(errBody))

            val ex = assertThrows(ForumException.NotFound::class.java) {
                runBlocking { client(server).getPostDetail("9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d") }
            }
            assertEquals("forum.not-found", ex.code)
        }
    }

    @Test
    fun `handles 400 bad request error envelope`() = runTest {
        MockWebServer().use { server ->
            val errBody = """{"error": {"code": "forum.bad-limit", "message": "invalid limit"}}"""
            server.enqueue(MockResponse().setResponseCode(400).setBody(errBody))

            val ex = assertThrows(ForumException.Invalid::class.java) {
                runBlocking { client(server).getFeed(limit = 100) }
            }
            assertEquals("forum.bad-limit", ex.code)
        }
    }

    @Test
    fun `handles 503 server unavailable`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setResponseCode(503).setBody("Service Unavailable"))

            val ex = assertThrows(ForumException.Unavailable::class.java) {
                runBlocking { client(server).getCategories() }
            }
            assertEquals("forum.unavailable", ex.code)
        }
    }

    @Test
    fun `enforces 512 KiB byte budget cap`() = runTest {
        MockWebServer().use { server ->
            val oversizedBody = "x".repeat(512 * 1024 + 10)
            server.enqueue(MockResponse().setResponseCode(200).setBody(oversizedBody))

            val ex = assertThrows(ForumException.Invalid::class.java) {
                runBlocking { client(server).getCategories() }
            }
            assertEquals("forum.over-budget", ex.code)
        }
    }

    @Test
    fun `surfaces connection failure as Offline exception`() = runTest {
        // Point client to closed port / non-responsive host
        val deadClient = ForumHttpClient(OkHttpClient(), "https://127.0.0.1:59999")
        val ex = assertThrows(ForumException.Offline::class.java) {
            runBlocking { deadClient.getCategories() }
        }
        assertEquals("forum.offline", ex.code)
    }
}
