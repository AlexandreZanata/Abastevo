package com.anpfuel.data.remote

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.repository.FeedbackException
import com.anpfuel.domain.repository.FeedbackRejectKind
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Assertions.fail
import org.junit.jupiter.api.Test

/**
 * P17-T02 RED: feedback transport against the wired P14 routes;
 * P22-T02 wires the ratings paths the same way.
 *
 * Writes carry the live session (family_id + access_token; the
 * author derives server-side). Missing/expired sessions never touch
 * the network ([FeedbackRejectKind.GATE_REQUIRED]). Backend codes
 * map to stable kinds (rating-out-of-range, stale-revision,
 * self-vote, quota-exceeded); IO failures and unmapped statuses are
 * TRANSPORT.
 */
class FeedbackHttpClientTest {

    private val session = PortableAuth.Session(
        familyId = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
        accountId = "acc-1",
        accessToken = "tok-access",
        refreshToken = "tok-refresh",
        accessExpiresAt = 9_999_999_999L,
        absoluteExpiresAt = 9_999_999_999L,
    )

    private class FakeSessions(var current: PortableAuth.Session?) : AuthSessionStore {
        override fun save(session: PortableAuth.Session) {
            current = session
        }

        override fun load(): PortableAuth.Session? = current

        override fun clear() {
            current = null
        }
    }

    private fun client(server: MockWebServer, sessions: FakeSessions): FeedbackHttpClient =
        FeedbackHttpClient(
            client = OkHttpClient.Builder()
                .connectTimeout(2L, TimeUnit.SECONDS)
                .readTimeout(2L, TimeUnit.SECONDS)
                .build(),
            baseUrl = server.url("/").toString(),
            sessions = sessions,
            nowEpochSeconds = { 1_700_000_000L },
        )

    private fun commentBody(id: String, revision: Int): String =
        """{"comment":{"id":"$id","public_alias":"adore-fox-42","station_id":"s-1","product":"GASOLINE","parent_id":null,"depth":0,"text":"good fuel","revision":$revision,"created_at":"2026-10-01T00:00:00Z","updated_at":"2026-10-01T00:00:00Z"}}"""

    @Test
    fun `comment posts session target text and parses receipt`() = runTest {
        val server = MockWebServer()
        try {
            server.enqueue(MockResponse().setResponseCode(201).setBody(commentBody("c-1", 1)))
            val receipt = client(server, FakeSessions(session))
                .submitComment("acc-1", "s-1", "GASOLINE", "good fuel")

            assertEquals("c-1", receipt.commentId)
            assertEquals(1, receipt.revision)
            val recorded = server.takeRequest()
            assertTrue(recorded.path!!.contains("/v1/feedback/comments"))
            val body = JSONObject(recorded.body.readUtf8())
            assertEquals(session.familyId, body.getString("family_id"))
            assertEquals("tok-access", body.getString("access_token"))
            assertEquals("s-1", body.getString("station_id"))
            assertEquals("good fuel", body.getString("text"))
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `missing session never touches the network`() = runTest {
        val server = MockWebServer()
        try {
            val gateway = client(server, FakeSessions(null))
            try {
                gateway.submitComment("acc-1", "s-1", "GASOLINE", "good fuel")
                fail("expected GATE_REQUIRED")
            } catch (refused: FeedbackException) {
                assertEquals(FeedbackRejectKind.GATE_REQUIRED, refused.kind)
            }
            assertEquals(0, server.requestCount)
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `edit conflict maps to stale revision`() = runTest {
        val server = MockWebServer()
        try {
            server.enqueue(
                MockResponse().setResponseCode(409)
                    .setBody("""{"code":"feedback.stale-revision","message":"comment changed"}"""),
            )
            try {
                client(server, FakeSessions(session)).editComment("acc-1", "c-1", "new text", 1)
                fail("expected STALE_REVISION")
            } catch (refused: FeedbackException) {
                assertEquals(FeedbackRejectKind.STALE_REVISION, refused.kind)
            }
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `self vote maps to denied kind`() = runTest {
        val server = MockWebServer()
        try {
            server.enqueue(
                MockResponse().setResponseCode(403)
                    .setBody("""{"code":"feedback.self-vote","message":"own comment"}"""),
            )
            try {
                client(server, FakeSessions(session)).vote("acc-1", "c-1", "VALID")
                fail("expected SELF_VOTE")
            } catch (refused: FeedbackException) {
                assertEquals(FeedbackRejectKind.SELF_VOTE, refused.kind)
            }
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `report quota maps to quota exceeded`() = runTest {
        val server = MockWebServer()
        try {
            server.enqueue(
                MockResponse().setResponseCode(429)
                    .setBody("""{"code":"feedback.quota-exceeded","message":"slow down"}"""),
            )
            try {
                client(server, FakeSessions(session)).report("acc-1", "c-1", "spam")
                fail("expected QUOTA_EXCEEDED")
            } catch (refused: FeedbackException) {
                assertEquals(FeedbackRejectKind.QUOTA_EXCEEDED, refused.kind)
            }
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `rate posts session target stars and parses receipt with stats`() = runTest {
        val server = MockWebServer()
        try {
            server.enqueue(
                MockResponse().setResponseCode(201).setBody(
                    """{"rating":{"station_id":"s-1","product":"GASOLINE","stars":5,"revision":1,"created":true},""" +
                        """"stats":{"station_id":"s-1","product":"GASOLINE","count":1,"sum":5,"mean_milli":5000}}""",
                ),
            )
            val receipt = client(server, FakeSessions(session))
                .rate("acc-1", "s-1", "GASOLINE", 5)

            assertTrue(receipt.created)
            assertEquals(1L, receipt.stats.count)
            assertEquals(5L, receipt.stats.sum)
            val recorded = server.takeRequest()
            assertTrue(recorded.path!!.contains("/v1/feedback/ratings"))
            val body = JSONObject(recorded.body.readUtf8())
            assertEquals(session.familyId, body.getString("family_id"))
            assertEquals(5, body.getInt("stars"))
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `out of range stars map to denied kind`() = runTest {
        val server = MockWebServer()
        try {
            server.enqueue(
                MockResponse().setResponseCode(400)
                    .setBody("""{"code":"feedback.rating-out-of-range","message":"invalid"}"""),
            )
            try {
                client(server, FakeSessions(session)).rate("acc-1", "s-1", "GASOLINE", 6)
                fail("expected RATING_OUT_OF_RANGE")
            } catch (refused: FeedbackException) {
                assertEquals(FeedbackRejectKind.RATING_OUT_OF_RANGE, refused.kind)
            }
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `stats get parses zeroed aggregate for unknown keys`() = runTest {
        val server = MockWebServer()
        try {
            server.enqueue(
                MockResponse().setResponseCode(200).setBody(
                    """{"stats":{"station_id":"s-9","product":"GASOLINE","count":0,"sum":0,"mean_milli":null}}""",
                ),
            )
            val stats = client(server, FakeSessions(session)).stats("s-9", "GASOLINE")
            assertEquals(0L, stats.count)
            assertEquals(0L, stats.sum)
            val recorded = server.takeRequest()
            assertTrue(recorded.path!!.contains("/v1/feedback/ratings/stats"))
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `second delete maps to rating not found`() = runTest {
        val server = MockWebServer()
        try {
            server.enqueue(
                MockResponse().setResponseCode(200).setBody("""{"status":"deleted"}"""),
            )
            client(server, FakeSessions(session)).deleteRating("acc-1", "s-1", "GASOLINE")
            server.enqueue(
                MockResponse().setResponseCode(404)
                    .setBody("""{"code":"feedback.rating-not-found","message":"gone"}"""),
            )
            try {
                client(server, FakeSessions(session)).deleteRating("acc-1", "s-1", "GASOLINE")
                fail("expected RATING_NOT_FOUND")
            } catch (refused: FeedbackException) {
                assertEquals(FeedbackRejectKind.RATING_NOT_FOUND, refused.kind)
            }
        } finally {
            server.shutdown()
        }
    }
}
