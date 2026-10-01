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
 * P17-T02 RED: feedback transport against the wired P14 routes.
 *
 * Writes carry the live session (family_id + access_token; the
 * author derives server-side). Missing/expired sessions never touch
 * the network ([FeedbackRejectKind.GATE_REQUIRED]). Backend codes
 * map to stable kinds (stale-revision, self-vote, quota-exceeded);
 * IO failures and unmapped statuses are TRANSPORT. Ratings have no
 * published path (recorded P14 gap) and refuse explicitly — never a
 * fake success.
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
    fun `ratings refuse without a published path`() = runTest {
        val server = MockWebServer()
        try {
            try {
                client(server, FakeSessions(session)).rate("acc-1", "s-1", "GASOLINE", 5)
                fail("expected GATE_REQUIRED")
            } catch (refused: FeedbackException) {
                assertEquals(FeedbackRejectKind.GATE_REQUIRED, refused.kind)
            }
            assertEquals(0, server.requestCount)
        } finally {
            server.shutdown()
        }
    }
}
