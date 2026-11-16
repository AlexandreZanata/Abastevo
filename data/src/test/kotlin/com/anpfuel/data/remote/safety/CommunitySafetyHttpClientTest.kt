package com.anpfuel.data.remote.safety

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.safety.CommunitySafetyException
import com.anpfuel.domain.safety.SafetyContentKind
import com.anpfuel.domain.safety.SafetyContentRef
import com.anpfuel.domain.safety.UserReportReason
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class CommunitySafetyHttpClientTest {

    private val commentId = "1c04a036-bc5d-48e9-89df-9648460de88d"
    private val topicId = "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d"
    private val blockId = "5f0c1a7e-7d7b-4d1e-9a51-6f1f3b2a9c10"
    private val session = PortableAuth.Session(
        familyId = "11111111-1111-4111-8111-111111111111",
        accountId = "22222222-2222-4222-8222-222222222222",
        accessToken = "test-access-token",
        refreshToken = "test-refresh-token",
        accessExpiresAt = Long.MAX_VALUE,
        absoluteExpiresAt = Long.MAX_VALUE,
    )

    private class Sessions(val current: PortableAuth.Session?) : AuthSessionStore {
        override fun save(session: PortableAuth.Session) = Unit
        override fun load(): PortableAuth.Session? = current
        override fun clear() = Unit
    }

    private class Signer {
        val paths = mutableListOf<String>()
        val bodies = mutableListOf<ByteArray>()
        val sign: suspend (String, ByteArray) -> Map<String, String> = { path, body ->
            paths += path
            bodies += body
            mapOf("Signature" to "sig", "Signature-Keyid" to "key", "Signature-Nonce" to "nonce")
        }
    }

    private fun client(
        server: MockWebServer,
        signer: Signer = Signer(),
        session: PortableAuth.Session? = this.session,
    ) = CommunitySafetyHttpClient(
        OkHttpClient(),
        server.url("/").toString(),
        sessions = Sessions(session),
        proofHeaders = signer.sign,
    )

    private fun ok(body: String) = MockResponse().setResponseCode(200).setBody(body)

    private fun error(status: Int, code: String) =
        MockResponse().setResponseCode(status).setBody("""{"error":{"code":"$code","message":"private server text"}}""")

    @Test
    fun `report user posts the signed envelope, reference, reason and detail`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(
                ok("""{"receipt":{"report_id":"$blockId","reason":"HARASSMENT","created_at":"2026-11-15T10:00:00Z","replayed":true}}"""),
            )
            val signer = Signer()
            val receipt = client(server, signer).reportUser(
                SafetyContentRef(SafetyContentKind.FORUM_COMMENT, commentId.uppercase()),
                UserReportReason.HARASSMENT,
                "detalhe",
            )
            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertEquals("POST", request.method)
            assertEquals("/v1/community/user-reports", request.path)
            assertEquals(listOf("/v1/community/user-reports"), signer.paths)
            assertEquals("sig", request.getHeader("Signature"))
            assertEquals("key", request.getHeader("Signature-Keyid"))
            assertEquals("no-store", request.getHeader("Cache-Control"))
            assertEquals("application/json", request.getHeader("Content-Type"))
            val raw = request.body.readByteArray()
            assertTrue(signer.bodies.single().contentEquals(raw), "the proof covers the exact body bytes")
            val body = JSONObject(String(raw, Charsets.UTF_8))
            assertEquals(session.familyId, body.getString("family_id"))
            assertEquals(session.accessToken, body.getString("access_token"))
            assertEquals("FORUM_COMMENT", body.getString("content_kind"))
            assertEquals(commentId, body.getString("content_id"))
            assertEquals("HARASSMENT", body.getString("reason"))
            assertEquals("detalhe", body.getString("detail"))
            assertEquals(blockId, receipt.reportId)
            assertEquals(UserReportReason.HARASSMENT, receipt.reason)
            assertTrue(receipt.replayed)
        }
    }

    @Test
    fun `report user without detail sends no detail field`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(ok("""{"receipt":{"report_id":"$blockId","reason":"SPAM","created_at":"2026-11-15T10:00:00Z","replayed":false}}"""))
            client(server).reportUser(SafetyContentRef(SafetyContentKind.FORUM_POST, topicId), UserReportReason.SPAM, null)
            val body = JSONObject(server.takeRequest(1, TimeUnit.SECONDS)!!.body.readUtf8())
            assertFalse(body.has("detail"))
            assertEquals("FORUM_POST", body.getString("content_kind"))
        }
    }

    @Test
    fun `block reads the block and the replay flag`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(
                ok("""{"block":{"block_id":"$blockId","label":"","source_kind":"STATION_COMMENT","created_at":"2026-11-15T10:00:00Z"},"replayed":true}"""),
            )
            val receipt = client(server).blockUser(SafetyContentRef(SafetyContentKind.STATION_COMMENT, commentId))
            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertEquals("/v1/community/blocks", request.path)
            val body = JSONObject(request.body.readUtf8())
            assertEquals("STATION_COMMENT", body.getString("content_kind"))
            assertEquals(commentId, body.getString("content_id"))
            assertEquals(blockId, receipt.blockedUser.blockId)
            assertEquals("", receipt.blockedUser.label)
            assertEquals(SafetyContentKind.STATION_COMMENT, receipt.blockedUser.sourceKind)
            assertTrue(receipt.replayed)
        }
    }

    @Test
    fun `list reads every block and keeps unknown source kinds`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(
                ok(
                    """{"items":[{"block_id":"$blockId","label":"Ana","source_kind":"FORUM_COMMENT","created_at":"2026-11-15T10:00:00Z"},""" +
                        """{"block_id":"$commentId","label":null,"source_kind":"CHAT","created_at":"2026-11-14T10:00:00Z"}]}""",
                ),
            )
            val signer = Signer()
            val items = client(server, signer).listBlocks()
            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertEquals("/v1/community/blocks/list", request.path)
            val body = JSONObject(request.body.readUtf8())
            assertEquals(setOf("family_id", "access_token"), body.keySet())
            assertEquals(listOf(blockId, commentId), items.map { it.blockId })
            assertEquals("Ana", items[0].label)
            assertEquals("", items[1].label)
            assertNull(items[1].sourceKind)
            assertEquals(listOf("/v1/community/blocks/list"), signer.paths)
        }
    }

    @Test
    fun `remove signs the block path and requires the removed flag`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(ok("""{"removed":true}"""))
            server.enqueue(ok("""{"removed":false}"""))
            val signer = Signer()
            val client = client(server, signer)
            client.removeBlock(blockId)
            assertEquals("/v1/community/blocks/$blockId/remove", server.takeRequest(1, TimeUnit.SECONDS)!!.path)
            assertEquals(listOf("/v1/community/blocks/$blockId/remove"), signer.paths)
            val refused = assertThrows(CommunitySafetyException.Unavailable::class.java) {
                runBlocking { client.removeBlock(blockId) }
            }
            assertEquals("safety.bad-response", refused.code)
        }
    }

    @Test
    fun `hidden sends the references and keeps only asked ones`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(
                ok(
                    """{"hidden":[{"content_kind":"FORUM_COMMENT","content_id":"${commentId.uppercase()}"},""" +
                        """{"content_kind":"FORUM_POST","content_id":"$blockId"},{"content_kind":"WHAT","content_id":"$commentId"}]}""",
                ),
            )
            val asked = listOf(
                SafetyContentRef(SafetyContentKind.FORUM_COMMENT, commentId),
                SafetyContentRef(SafetyContentKind.FORUM_POST, topicId),
            )
            val hidden = client(server).hiddenContent(asked)
            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertEquals("/v1/community/blocks/hidden", request.path)
            val items = JSONObject(request.body.readUtf8()).getJSONArray("items")
            assertEquals(2, items.length())
            assertEquals("FORUM_COMMENT", items.getJSONObject(0).getString("content_kind"))
            assertEquals(commentId, items.getJSONObject(0).getString("content_id"))
            assertEquals(setOf(asked[0]), hidden)
        }
    }

    @Test
    fun `hidden asks nothing for no references and refuses more than one hundred`() = runTest {
        MockWebServer().use { server ->
            val client = client(server)
            assertTrue(client.hiddenContent(emptyList()).isEmpty())
            val tooMany = (0 until 101).map {
                SafetyContentRef(SafetyContentKind.FORUM_COMMENT, "00000000-0000-4000-8000-%012d".format(it))
            }
            assertThrows(CommunitySafetyException.BadRequest::class.java) { runBlocking { client.hiddenContent(tooMany) } }
            assertEquals(0, server.requestCount)
        }
    }

    @Test
    fun `no live session sends nothing`() = runTest {
        MockWebServer().use { server ->
            val client = client(server, session = null)
            assertThrows(CommunitySafetyException.AuthRequired::class.java) { runBlocking { client.listBlocks() } }
            val expired = client(server, session = session.copy(accessExpiresAt = 1))
            assertThrows(CommunitySafetyException.AuthRequired::class.java) {
                runBlocking { expired.blockUser(SafetyContentRef(SafetyContentKind.FORUM_COMMENT, commentId)) }
            }
            val noStore = CommunitySafetyHttpClient(OkHttpClient(), server.url("/").toString())
            assertThrows(CommunitySafetyException.AuthRequired::class.java) { runBlocking { noStore.listBlocks() } }
            assertEquals(0, server.requestCount)
        }
    }

    @Test
    fun `an unavailable request proof sends nothing`() = runTest {
        MockWebServer().use { server ->
            val client = CommunitySafetyHttpClient(
                OkHttpClient(),
                server.url("/").toString(),
                sessions = Sessions(session),
                proofHeaders = { _, _ -> throw java.io.IOException("photo.identity-unavailable") },
            )
            val refused = assertThrows(CommunitySafetyException.Unavailable::class.java) { runBlocking { client.listBlocks() } }
            assertEquals("safety.signing-unavailable", refused.code)
            assertEquals(0, server.requestCount)
        }
    }

    @Test
    fun `error envelopes map to typed refusals without server text`() = runTest {
        MockWebServer().use { server ->
            val ref = SafetyContentRef(SafetyContentKind.FORUM_COMMENT, commentId)
            val cases = listOf(
                error(400, "safety.self-target") to CommunitySafetyException.SelfTarget::class.java,
                error(400, "safety.bad-request") to CommunitySafetyException.BadRequest::class.java,
                error(401, "safety.auth-required") to CommunitySafetyException.AuthRequired::class.java,
                error(404, "safety.not-found") to CommunitySafetyException.NotFound::class.java,
                error(409, "safety.block-limit") to CommunitySafetyException.BlockLimit::class.java,
                error(503, "safety.unavailable") to CommunitySafetyException.Unavailable::class.java,
                MockResponse().setResponseCode(500).setBody("<html>oops</html>") to CommunitySafetyException.Unavailable::class.java,
            )
            val client = client(server)
            for ((response, type) in cases) {
                server.enqueue(response)
                val thrown = assertThrows(CommunitySafetyException::class.java) { runBlocking { client.blockUser(ref) } }
                assertTrue(type.isInstance(thrown), "expected ${type.simpleName}, got ${thrown::class.simpleName}")
                assertFalse(thrown.message!!.contains("private server text"))
                assertTrue(thrown.code.startsWith("safety."))
            }
        }
    }

    @Test
    fun `a 400 with a foreign code keeps a safe code`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setResponseCode(400).setBody("""{"error":{"code":"<script>","message":"x"}}"""))
            val refused = assertThrows(CommunitySafetyException.BadRequest::class.java) {
                runBlocking { client(server).listBlocks() }
            }
            assertEquals("safety.bad-request", refused.code)
        }
    }

    @Test
    fun `rate limits carry retry after seconds when valid`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(error(429, "safety.rate-limited").setHeader("Retry-After", "120"))
            server.enqueue(error(429, "safety.rate-limited").setHeader("Retry-After", "soon"))
            val client = client(server)
            val ref = SafetyContentRef(SafetyContentKind.FORUM_POST, topicId)
            val first = assertThrows(CommunitySafetyException.RateLimited::class.java) {
                runBlocking { client.reportUser(ref, UserReportReason.SPAM, null) }
            }
            assertEquals(120L, first.retryAfterSeconds)
            val second = assertThrows(CommunitySafetyException.RateLimited::class.java) {
                runBlocking { client.reportUser(ref, UserReportReason.SPAM, null) }
            }
            assertNull(second.retryAfterSeconds)
        }
    }

    @Test
    fun `malformed success bodies are refused`() = runTest {
        MockWebServer().use { server ->
            val ref = SafetyContentRef(SafetyContentKind.FORUM_COMMENT, commentId)
            val client = client(server)
            server.enqueue(ok("not json"))
            assertThrows(CommunitySafetyException.Unavailable::class.java) { runBlocking { client.blockUser(ref) } }
            server.enqueue(ok("""{"block":{"label":"Ana","created_at":"2026-11-15T10:00:00Z"},"replayed":false}"""))
            assertThrows(CommunitySafetyException.Unavailable::class.java) { runBlocking { client.blockUser(ref) } }
            server.enqueue(ok("""{"receipt":{"report_id":"","created_at":"x"}}"""))
            assertThrows(CommunitySafetyException.Unavailable::class.java) {
                runBlocking { client.reportUser(ref, UserReportReason.SPAM, null) }
            }
            server.enqueue(ok("""{"items":{}}"""))
            assertThrows(CommunitySafetyException.Unavailable::class.java) { runBlocking { client.listBlocks() } }
            server.enqueue(ok("""{"hidden":"all"}"""))
            assertThrows(CommunitySafetyException.Unavailable::class.java) { runBlocking { client.hiddenContent(listOf(ref)) } }
        }
    }

    @Test
    fun `oversized bodies are refused unread`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(ok("x".repeat(256 * 1024 + 10)))
            val refused = assertThrows(CommunitySafetyException.Unavailable::class.java) {
                runBlocking { client(server).listBlocks() }
            }
            assertEquals("safety.over-budget", refused.code)
        }
    }

    @Test
    fun `connection failures are offline`() = runTest {
        val dead = CommunitySafetyHttpClient(OkHttpClient(), "https://127.0.0.1:59999", sessions = Sessions(session))
        assertThrows(CommunitySafetyException.Offline::class.java) { runBlocking { dead.listBlocks() } }
    }
}
