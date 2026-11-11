package com.anpfuel.data.remote.forum

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumLiveEvent
import com.anpfuel.domain.forum.ForumReaction
import com.anpfuel.domain.forum.ForumReactionCounts
import com.anpfuel.domain.forum.ForumReactionTarget
import com.anpfuel.domain.forum.ForumReplySort
import com.anpfuel.domain.portable.PortableAuth
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ForumSocialClientTest {

    private val postId = "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d"
    private val commentId = "1c04a036-bc5d-48e9-89df-9648460de88d"
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

    private fun signedClient(server: MockWebServer, signed: MutableList<String>) = ForumHttpClient(
        OkHttpClient(),
        server.url("/").toString(),
        sessions = Sessions(session),
        proofHeaders = { path, _ ->
            signed += path
            mapOf("Signature" to "sig", "Signature-Keyid" to "key", "Signature-Nonce" to "nonce")
        },
    )

    @Test
    fun `ranked comments send the sort and read counts and the best answer`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(
                MockResponse().setResponseCode(200).setBody(
                    """{"items":[{"id":"$commentId","post_id":"$postId","depth":0,"sequence":4,"revision":2,
                    "public_alias":"Motorista","body_text":"Etanol compensa","created_at":"2026-11-11T10:00:00Z",
                    "reply_count":2,"like_count":7,"dislike_count":1,"top_reply":{"id":"2c04a036-bc5d-48e9-89df-9648460de88d",
                    "post_id":"$postId","parent_id":"$commentId","depth":1,"sequence":6,"revision":2,"public_alias":"Ana",
                    "body_text":"Concordo","created_at":"2026-11-11T10:05:00Z","reply_count":0,"like_count":3,"dislike_count":0}}],
                    "next_cursor":"6.4"}""",
                ),
            )
            val page = ForumHttpClient(OkHttpClient(), server.url("/").toString())
                .getRankedComments(postId, ForumReplySort.RELEVANT, 20, "9.12")
            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertTrue(request.path!!.contains("sort=relevant"))
            assertTrue(request.path!!.contains("cursor=9.12"))
            val first = page.items.single()
            assertEquals(ForumReactionCounts(7, 1), first.reactions)
            assertEquals("Concordo", first.topReply!!.bodyText)
            assertEquals(ForumReactionCounts(3, 0), first.topReply!!.reactions)
            assertNull(first.topReply!!.topReply)
            assertEquals("6.4", page.nextCursor)
        }
    }

    @Test
    fun `reactions are signed session commands with the chosen value`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(
                MockResponse().setResponseCode(200).setBody(
                    """{"target_type":"COMMENT","target_id":"$commentId","reaction":"LIKE","like_count":8,"dislike_count":1,"replayed":false}""",
                ),
            )
            val signed = mutableListOf<String>()
            val result = signedClient(server, signed).setReaction(
                ForumReactionTarget.COMMENT,
                commentId,
                ForumReaction.LIKE,
                "33333333-3333-4333-8333-333333333333",
            )
            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertEquals("/v1/forum/comments/$commentId/reaction", request.path)
            assertEquals(listOf("/v1/forum/comments/$commentId/reaction"), signed)
            assertEquals("sig", request.getHeader("Signature"))
            val body = JSONObject(request.body.readUtf8())
            assertEquals("LIKE", body.getString("reaction"))
            assertEquals(session.familyId, body.getString("family_id"))
            assertEquals("33333333-3333-4333-8333-333333333333", body.getString("operation_id"))
            assertEquals(ForumReaction.LIKE, result.reaction)
            assertEquals(ForumReactionCounts(8, 1), result.counts)
        }
    }

    @Test
    fun `reactions need a live session and send nothing without one`() = runTest {
        MockWebServer().use { server ->
            val client = ForumHttpClient(OkHttpClient(), server.url("/").toString(), sessions = Sessions(null))
            assertThrows(ForumException.AuthRequired::class.java) {
                runBlocking {
                    client.setReaction(ForumReactionTarget.POST, postId, ForumReaction.DISLIKE, "33333333-3333-4333-8333-333333333333")
                }
            }
            assertEquals(0, server.requestCount)
        }
    }

    @Test
    fun `own reactions are a signed owner read`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(
                MockResponse().setResponseCode(200).setBody(
                    """{"post_reaction":"DISLIKE","comments":[{"comment_id":"$commentId","reaction":"LIKE"}]}""",
                ),
            )
            val signed = mutableListOf<String>()
            val own = signedClient(server, signed).getOwnReactions(postId)
            assertEquals(listOf("/v1/forum/owner/reactions"), signed)
            assertEquals(ForumReaction.DISLIKE, own.post)
            assertEquals(mapOf(commentId to ForumReaction.LIKE), own.comments)
        }
    }

    @Test
    fun `sse parser joins frames and skips comments`() {
        val parser = ForumSseParser()
        val frames = listOf("retry: 3000", "event: ready", "data: {\"post_id\":\"x\"}", "", ": ping", "",
            "event: comment_gone", "data: {\"comment_id\":\"c\"}", "").mapNotNull { parser.feed(it) }
        assertEquals(listOf("ready" to "{\"post_id\":\"x\"}", "comment_gone" to "{\"comment_id\":\"c\"}"), frames)
    }

    @Test
    fun `live stream emits typed events in order and ends on reconnect`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(
                MockResponse().setResponseCode(200).setHeader("Content-Type", "text/event-stream").setBody(
                    "retry: 3000\nevent: ready\ndata: {\"post_id\":\"$postId\"}\n\n" +
                        ": ping\n\n" +
                        "event: comment\ndata: {\"comment\":{\"id\":\"$commentId\",\"post_id\":\"$postId\",\"depth\":0," +
                        "\"sequence\":9,\"revision\":2,\"public_alias\":\"Bia\",\"body_text\":\"Novo\"," +
                        "\"created_at\":\"2026-11-11T11:00:00Z\",\"reply_count\":0,\"like_count\":0,\"dislike_count\":0}}\n\n" +
                        "event: comment_reactions\ndata: {\"comment_id\":\"$commentId\",\"like_count\":2,\"dislike_count\":1}\n\n" +
                        "event: future_event\ndata: {}\n\n" +
                        "event: post_reactions\ndata: {\"like_count\":5,\"dislike_count\":0}\n\n" +
                        "event: reconnect\ndata: {}\n\n" +
                        "event: comment_gone\ndata: {\"comment_id\":\"after-reconnect\"}\n\n",
                ),
            )
            val events = ForumLiveClient(OkHttpClient(), server.url("/").toString()).conversation(postId).toList()
            assertEquals("/v1/forum/posts/$postId/stream", server.takeRequest(1, TimeUnit.SECONDS)!!.path)
            assertEquals(ForumLiveEvent.Ready, events[0])
            assertEquals("Novo", (events[1] as ForumLiveEvent.CommentPublished).comment.bodyText)
            assertEquals(ForumLiveEvent.CommentReactions(commentId, null, ForumReactionCounts(2, 1)), events[2])
            assertEquals(ForumLiveEvent.PostReactions(ForumReactionCounts(5, 0)), events[3])
            assertEquals(ForumLiveEvent.Reconnect, events[4])
            assertEquals(5, events.size)
        }
    }

    @Test
    fun `live stream reports a hidden post as not found`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setResponseCode(404).setBody("""{"error":{"code":"forum.not-found"}}"""))
            assertThrows(ForumException.NotFound::class.java) {
                runBlocking { ForumLiveClient(OkHttpClient(), server.url("/").toString()).conversation(postId).toList() }
            }
        }
    }
}
