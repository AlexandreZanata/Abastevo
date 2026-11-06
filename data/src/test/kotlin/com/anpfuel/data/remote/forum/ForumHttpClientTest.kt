package com.anpfuel.data.remote.forum

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumFeedFilters
import com.anpfuel.domain.forum.ForumPostKind
import com.anpfuel.domain.forum.ForumRegion
import com.anpfuel.domain.portable.PortableAuth
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
import org.json.JSONObject

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

    private val testSession = PortableAuth.Session(
        familyId = "11111111-1111-4111-8111-111111111111",
        accountId = "22222222-2222-4222-8222-222222222222",
        accessToken = "test-access-token",
        refreshToken = "test-refresh-token",
        accessExpiresAt = Long.MAX_VALUE,
        absoluteExpiresAt = Long.MAX_VALUE,
    )

    private class TestSessionStore(var current: PortableAuth.Session? = null) : AuthSessionStore {
        override fun save(session: PortableAuth.Session) { current = session }
        override fun load(): PortableAuth.Session? = current
        override fun clear() { current = null }
    }

    private fun client(server: MockWebServer, sessions: AuthSessionStore? = null) =
        ForumHttpClient(OkHttpClient(), server.url("/").toString(), sessions = sessions)

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
                filters = ForumFeedFilters(
                    uf = "SP",
                    municipalityIbge = 3550308,
                    fuelProduct = "ETHANOL",
                    stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26",
                ),
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
            assertTrue(request.path!!.contains("uf=SP"))
            assertTrue(request.path!!.contains("municipality_ibge=3550308"))
            assertTrue(request.path!!.contains("fuel_product=ETHANOL"))
            assertTrue(request.path!!.contains("station_id=d6c74c23-63db-4c24-a2e5-408cb23bad26"))
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

    @Test
    fun `getComments sends GET by post ID and parses comments`() = runTest {
        MockWebServer().use { server ->
            val commentsJson = """
                {
                    "items": [
                        {
                            "id": "11111111-1111-4111-8111-111111111111",
                            "post_id": "22222222-2222-4222-8222-222222222222",
                            "parent_comment_id": null,
                            "author_public_alias": "Motorista SP",
                            "body_text": "Ótimo post.",
                            "created_at": "2026-11-05T14:00:00Z",
                            "updated_at": "2026-11-05T14:00:00Z",
                            "revision": 1,
                            "replies_count": 0,
                            "is_author": false
                        }
                    ],
                    "next_cursor": "cursor-abc"
                }
            """.trimIndent()
            server.enqueue(MockResponse().setResponseCode(200).setBody(commentsJson))
            val result = client(server).getComments(
                postId = "22222222-2222-4222-8222-222222222222",
                limit = 10,
                cursor = "cursor-start",
            )
            assertEquals(1, result.items.size)
            assertEquals("11111111-1111-4111-8111-111111111111", result.items[0].id)
            assertEquals("cursor-abc", result.nextCursor)

            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertEquals("GET", request.method)
            assertTrue(request.path!!.contains("/v1/forum/posts/22222222-2222-4222-8222-222222222222/comments"))
            assertTrue(request.path!!.contains("limit=10"))
            assertTrue(request.path!!.contains("cursor=cursor-start"))
        }
    }

    @Test
    fun `getReplies sends GET by comment ID and parses replies`() = runTest {
        MockWebServer().use { server ->
            val repliesJson = """
                {
                    "items": [
                        {
                            "id": "33333333-3333-4333-8333-333333333333",
                            "post_id": "22222222-2222-4222-8222-222222222222",
                            "parent_comment_id": "11111111-1111-4111-8111-111111111111",
                            "author_public_alias": "Motorista RJ",
                            "body_text": "Resposta útil.",
                            "created_at": "2026-11-05T14:05:00Z",
                            "updated_at": "2026-11-05T14:05:00Z",
                            "revision": 1,
                            "replies_count": 0,
                            "is_author": true
                        }
                    ],
                    "next_cursor": null
                }
            """.trimIndent()
            server.enqueue(MockResponse().setResponseCode(200).setBody(repliesJson))
            val result = client(server).getReplies(
                commentId = "11111111-1111-4111-8111-111111111111",
                limit = 5,
            )
            assertEquals(1, result.items.size)
            assertEquals("33333333-3333-4333-8333-333333333333", result.items[0].id)

            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertEquals("GET", request.method)
            assertTrue(request.path!!.contains("/v1/forum/comments/11111111-1111-4111-8111-111111111111/replies"))
            assertTrue(request.path!!.contains("limit=5"))
        }
    }

    @Test
    fun `createComment sends POST with session credentials and body`() = runTest {
        MockWebServer().use { server ->
            val receiptJson = """
                {
                    "id": "55555555-5555-4555-8555-555555555555",
                    "post_id": "22222222-2222-4222-8222-222222222222",
                    "parent_id": null,
                    "depth": 0,
                    "sequence": 1,
                    "revision": 1,
                    "publication_state": "PENDING_REVIEW",
                    "visibility": "VISIBLE",
                    "body_text": "Comentário de teste.",
                    "created_at": "2026-11-05T14:15:00Z",
                    "updated_at": "2026-11-05T14:15:00Z"
                }
            """.trimIndent()
            server.enqueue(MockResponse().setResponseCode(200).setBody(receiptJson))
            val sessions = TestSessionStore(testSession)
            val receipt = client(server, sessions).createComment(
                postId = "22222222-2222-4222-8222-222222222222",
                parentId = null,
                bodyText = "Comentário de teste.",
                operationId = "op-create-1",
            )
            assertEquals("55555555-5555-4555-8555-555555555555", receipt.id)
            assertEquals(1L, receipt.revision)
            assertEquals("PENDING_REVIEW", receipt.publicationState)

            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertEquals("POST", request.method)
            assertEquals("/v1/forum/posts/22222222-2222-4222-8222-222222222222/comments", request.path)
            val json = JSONObject(request.body.readUtf8())
            assertEquals("test-access-token", json.getString("access_token"))
            assertEquals("Comentário de teste.", json.getString("body_text"))
            assertEquals("op-create-1", json.getString("operation_id"))
        }
    }

    @Test
    fun `editComment sends POST with revision and body`() = runTest {
        MockWebServer().use { server ->
            val receiptJson = """
                {
                    "id": "55555555-5555-4555-8555-555555555555",
                    "post_id": "22222222-2222-4222-8222-222222222222",
                    "parent_id": null,
                    "depth": 0,
                    "sequence": 1,
                    "revision": 2,
                    "publication_state": "PENDING_REVIEW",
                    "visibility": "VISIBLE",
                    "body_text": "Texto atualizado.",
                    "created_at": "2026-11-05T14:20:00Z",
                    "updated_at": "2026-11-05T14:20:00Z"
                }
            """.trimIndent()
            server.enqueue(MockResponse().setResponseCode(200).setBody(receiptJson))
            val sessions = TestSessionStore(testSession)
            val receipt = client(server, sessions).editComment(
                commentId = "55555555-5555-4555-8555-555555555555",
                expectedRevision = 1L,
                bodyText = "Texto atualizado.",
                operationId = "op-edit-1",
            )
            assertEquals(2L, receipt.revision)

            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertEquals("POST", request.method)
            assertEquals("/v1/forum/comments/55555555-5555-4555-8555-555555555555/edit", request.path)
            val json = JSONObject(request.body.readUtf8())
            assertEquals(1L, json.getLong("expected_revision"))
            assertEquals("Texto atualizado.", json.getString("body_text"))
        }
    }

    @Test
    fun `removeComment sends POST with revision`() = runTest {
        MockWebServer().use { server ->
            val receiptJson = """
                {
                    "id": "55555555-5555-4555-8555-555555555555",
                    "post_id": "22222222-2222-4222-8222-222222222222",
                    "parent_id": null,
                    "depth": 0,
                    "sequence": 1,
                    "revision": 3,
                    "publication_state": "REMOVED",
                    "visibility": "HIDDEN",
                    "body_text": "",
                    "created_at": "2026-11-05T14:25:00Z",
                    "updated_at": "2026-11-05T14:25:00Z"
                }
            """.trimIndent()
            server.enqueue(MockResponse().setResponseCode(200).setBody(receiptJson))
            val sessions = TestSessionStore(testSession)
            val receipt = client(server, sessions).removeComment(
                commentId = "55555555-5555-4555-8555-555555555555",
                expectedRevision = 2L,
                operationId = "op-remove-1",
            )
            assertEquals("REMOVED", receipt.publicationState)

            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertEquals("POST", request.method)
            assertEquals("/v1/forum/comments/55555555-5555-4555-8555-555555555555/remove", request.path)
            val json = JSONObject(request.body.readUtf8())
            assertEquals(2L, json.getLong("expected_revision"))
        }
    }

    @Test
    fun `report sends POST with reason and detail`() = runTest {
        MockWebServer().use { server ->
            val receiptJson = """
                {
                    "report_id": "66666666-6666-4666-8666-666666666666",
                    "case_id": "case-999",
                    "target_type": "FORUM_COMMENT",
                    "target_id": "55555555-5555-4555-8555-555555555555",
                    "reason": "SPAM",
                    "status": "OPEN",
                    "created_at": "2026-11-05T14:30:00Z"
                }
            """.trimIndent()
            server.enqueue(MockResponse().setResponseCode(200).setBody(receiptJson))
            val sessions = TestSessionStore(testSession)
            val receipt = client(server, sessions).report(
                targetType = "FORUM_COMMENT",
                targetId = "55555555-5555-4555-8555-555555555555",
                reason = "SPAM",
                detail = "Spam promocional.",
            )
            assertEquals("66666666-6666-4666-8666-666666666666", receipt.reportId)

            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertEquals("POST", request.method)
            assertEquals("/v1/forum/comments/55555555-5555-4555-8555-555555555555/report", request.path)
            val json = JSONObject(request.body.readUtf8())
            assertEquals("SPAM", json.getString("reason"))
            assertEquals("Spam promocional.", json.getString("detail"))
        }
    }

    @Test
    fun `handles 401 auth required`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setResponseCode(401).setBody("""{"error": {"code": "forum.unauthorized"}}"""))
            val sessions = TestSessionStore(testSession)
            val ex = assertThrows(ForumException.AuthRequired::class.java) {
                runBlocking {
                    client(server, sessions).createComment("22222222-2222-4222-8222-222222222222", null, "teste", "op-err")
                }
            }
            assertEquals("forum.auth-required", ex.code)
        }
    }

    @Test
    fun `handles 409 conflict`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setResponseCode(409).setBody("""{"error": {"code": "forum.conflict"}}"""))
            val sessions = TestSessionStore(testSession)
            val ex = assertThrows(ForumException.Conflict::class.java) {
                runBlocking {
                    client(server, sessions).editComment("55555555-5555-4555-8555-555555555555", 1L, "novo texto", "op-conf")
                }
            }
            assertEquals("forum.conflict", ex.code)
        }
    }

    @Test
    fun `handles 429 rate limit with Retry-After header`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(
                MockResponse()
                    .setResponseCode(429)
                    .setHeader("Retry-After", "30")
                    .setBody("""{"error": {"code": "forum.rate-limited", "message": "rate limit exceeded"}}""")
            )
            val sessions = TestSessionStore(testSession)
            val ex = assertThrows(ForumException.RateLimited::class.java) {
                runBlocking {
                    client(server, sessions).createComment("22222222-2222-4222-8222-222222222222", null, "teste", "op-rate")
                }
            }
            assertEquals("forum.rate-limited", ex.code)
            assertEquals(30L, ex.retryAfterSeconds)
        }
    }

    @Test
    fun `createTopic sends signed payload and receives topic receipt`() = runTest {
        MockWebServer().use { server ->
            val topicResponseJson = """
                {
                    "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
                    "kind": "TOPIC",
                    "category": "FUELS",
                    "title": "Gasolina em SP",
                    "body_text": "Postos na zona sul",
                    "revision": 1,
                    "publication_state": "PENDING_REVIEW",
                    "visibility": "VISIBLE",
                    "region": {
                        "uf": "SP",
                        "municipality_ibge": 3550308
                    },
                    "fuel_product": "GASOLINE_REGULAR",
                    "created_at": "2026-11-06T10:00:00Z",
                    "updated_at": "2026-11-06T10:00:00Z",
                    "replayed": false
                }
            """.trimIndent()
            server.enqueue(MockResponse().setResponseCode(201).setBody(topicResponseJson))
            val sessions = TestSessionStore(testSession)
            val receipt = client(server, sessions).createTopic(
                title = "Gasolina em SP",
                bodyText = "Postos na zona sul",
                category = ForumCategory.FUELS,
                operationId = "33333333-3333-4333-8333-333333333333",
                region = ForumRegion("SP", 3550308),
                fuelProduct = "GASOLINE_REGULAR",
                stationId = null,
                publicAlias = "Alex",
            )
            val recorded = server.takeRequest(1, TimeUnit.SECONDS)
            assertEquals("/v1/forum/topics", recorded?.path)
            assertEquals("POST", recorded?.method)
            val recordedJson = JSONObject(recorded?.body?.readUtf8().orEmpty())
            assertEquals(testSession.familyId, recordedJson.getString("family_id"))
            assertEquals(testSession.accessToken, recordedJson.getString("access_token"))
            assertEquals("FUELS", recordedJson.getString("category"))
            assertEquals("Gasolina em SP", recordedJson.getString("title"))
            assertEquals("SP", recordedJson.getJSONObject("region").getString("uf"))
            assertEquals("3fa85f64-5717-4562-b3fc-2c963f66afa6", receipt.id)
            assertEquals("PENDING_REVIEW", receipt.publicationState)
        }
    }

    @Test
    fun `editTopic sends signed payload with revision and receives updated receipt`() = runTest {
        MockWebServer().use { server ->
            val topicResponseJson = """
                {
                    "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
                    "kind": "TOPIC",
                    "category": "MARKET",
                    "title": "Gasolina em SP - Atualizado",
                    "body_text": "Postos na zona sul e centro",
                    "revision": 2,
                    "publication_state": "PENDING_REVIEW",
                    "visibility": "VISIBLE",
                    "created_at": "2026-11-06T10:00:00Z",
                    "updated_at": "2026-11-06T10:30:00Z",
                    "replayed": false
                }
            """.trimIndent()
            server.enqueue(MockResponse().setResponseCode(200).setBody(topicResponseJson))
            val sessions = TestSessionStore(testSession)
            val receipt = client(server, sessions).editTopic(
                topicId = "3fa85f64-5717-4562-b3fc-2c963f66afa6",
                expectedRevision = 1L,
                title = "Gasolina em SP - Atualizado",
                bodyText = "Postos na zona sul e centro",
                category = ForumCategory.MARKET,
                operationId = "44444444-4444-4444-8444-444444444444",
            )
            val recorded = server.takeRequest(1, TimeUnit.SECONDS)
            assertEquals("/v1/forum/topics/3fa85f64-5717-4562-b3fc-2c963f66afa6/edit", recorded?.path)
            assertEquals("POST", recorded?.method)
            val recordedJson = JSONObject(recorded?.body?.readUtf8().orEmpty())
            assertEquals(1L, recordedJson.getLong("expected_revision"))
            assertEquals("MARKET", recordedJson.getString("category"))
            assertEquals(2L, receipt.revision)
        }
    }

    @Test
    fun `removeTopic sends signed payload and receives removed receipt`() = runTest {
        MockWebServer().use { server ->
            val topicResponseJson = """
                {
                    "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
                    "kind": "TOPIC",
                    "category": "FUELS",
                    "title": "[Tópico removido]",
                    "body_text": "",
                    "revision": 3,
                    "publication_state": "REMOVED",
                    "visibility": "HIDDEN",
                    "created_at": "2026-11-06T10:00:00Z",
                    "updated_at": "2026-11-06T11:00:00Z",
                    "replayed": false
                }
            """.trimIndent()
            server.enqueue(MockResponse().setResponseCode(200).setBody(topicResponseJson))
            val sessions = TestSessionStore(testSession)
            val receipt = client(server, sessions).removeTopic(
                topicId = "3fa85f64-5717-4562-b3fc-2c963f66afa6",
                operationId = "55555555-5555-4555-8555-555555555555",
            )
            val recorded = server.takeRequest(1, TimeUnit.SECONDS)
            assertEquals("/v1/forum/topics/3fa85f64-5717-4562-b3fc-2c963f66afa6/remove", recorded?.path)
            assertEquals("POST", recorded?.method)
            assertEquals("REMOVED", receipt.publicationState)
            assertEquals("HIDDEN", receipt.visibility)
        }
    }

    @Test
    fun `getOwnerTopic sends signed topic_id and returns owner projection`() = runTest {
        MockWebServer().use { server ->
            val ownerTopicResponseJson = """
                {
                    "topic": {
                        "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
                        "kind": "TOPIC",
                        "category": "FUELS",
                        "title": "Gasolina em SP",
                        "body_text": "Postos na zona sul",
                        "revision": 1,
                        "publication_state": "PENDING_REVIEW",
                        "visibility": "VISIBLE",
                        "created_at": "2026-11-06T10:00:00Z",
                        "updated_at": "2026-11-06T10:00:00Z",
                        "replayed": false
                    }
                }
            """.trimIndent()
            server.enqueue(MockResponse().setResponseCode(200).setBody(ownerTopicResponseJson))
            val sessions = TestSessionStore(testSession)
            val receipt = client(server, sessions).getOwnerTopic("3fa85f64-5717-4562-b3fc-2c963f66afa6")
            val recorded = server.takeRequest(1, TimeUnit.SECONDS)
            assertEquals("/v1/forum/owner/read", recorded?.path)
            assertEquals("POST", recorded?.method)
            val recordedJson = JSONObject(recorded?.body?.readUtf8().orEmpty())
            assertEquals("3fa85f64-5717-4562-b3fc-2c963f66afa6", recordedJson.getString("topic_id"))
            assertEquals("3fa85f64-5717-4562-b3fc-2c963f66afa6", receipt.id)
            assertEquals("PENDING_REVIEW", receipt.publicationState)
        }
    }
}
