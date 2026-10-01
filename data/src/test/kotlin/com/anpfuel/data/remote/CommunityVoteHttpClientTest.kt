package com.anpfuel.data.remote

import com.anpfuel.domain.repository.CommunityVoteException
import com.anpfuel.domain.repository.CommunityVoteRejectKind
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
 * P10-T07: vote paths, bodies and typed error mapping.
 *
 * Private detail travels only in the dispute body and never appears
 * in error text; identical retries keep converging server-side while
 * this client performs a single POST per call.
 */
class CommunityVoteHttpClientTest {

    private val observationId = "11111111-1111-4111-8111-111111111111"

    private fun client(server: MockWebServer): CommunityVoteHttpClient =
        CommunityVoteHttpClient(
            client = OkHttpClient.Builder()
                .connectTimeout(2L, TimeUnit.SECONDS)
                .readTimeout(2L, TimeUnit.SECONDS)
                .build(),
            baseUrl = server.url("/").toString(),
        )

    @Test
    fun `confirm posts stable submission id and parses vote id`() = runTest {
        val server = MockWebServer()
        try {
            server.enqueue(
                MockResponse().setResponseCode(201)
                    .setBody("""{"confirmation_id":"vote-1","observation_id":"$observationId"}"""),
            )
            val receipt = client(server).submitConfirmation(observationId, "cmd-1")

            assertEquals("vote-1", receipt.voteId)
            assertEquals(observationId, receipt.observationId)
            val recorded = server.takeRequest()
            assertTrue(recorded.path!!.contains("/v1/observations/$observationId/confirmations"))
            val body = JSONObject(recorded.body.readUtf8())
            assertEquals("cmd-1", body.getString("client_submission_id"))
            assertEquals("cmd-1", recorded.getHeader("Idempotency-Key"))
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `dispute posts reason detail and replacement`() = runTest {
        val server = MockWebServer()
        try {
            server.enqueue(
                MockResponse().setResponseCode(201)
                    .setBody("""{"dispute_id":"dispute-7","state":"OPEN"}"""),
            )
            val receipt = client(server).submitDispute(
                observationId,
                "cmd-2",
                "PRICE_CHANGED",
                "pump shows 6.19",
                "22222222-2222-4222-8222-222222222222",
            )

            assertEquals("dispute-7", receipt.voteId)
            val recorded = server.takeRequest()
            assertTrue(recorded.path!!.contains("/v1/observations/$observationId/disputes"))
            val body = JSONObject(recorded.body.readUtf8())
            assertEquals("PRICE_CHANGED", body.getString("reason"))
            assertEquals("pump shows 6.19", body.getString("detail"))
            assertEquals(
                "22222222-2222-4222-8222-222222222222",
                body.getString("replacement_observation_id"),
            )
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `self-confirm maps to typed denial without leaking detail`() = runTest {
        val server = MockWebServer()
        try {
            server.enqueue(
                MockResponse().setResponseCode(403)
                    .setBody("""{"code":"community.self-confirmation"}"""),
            )
            try {
                client(server).submitConfirmation(observationId, "cmd-1")
                fail("expected CommunityVoteException")
            } catch (error: CommunityVoteException) {
                assertEquals(CommunityVoteRejectKind.SELF_CONFIRMATION, error.kind)
                assertTrue(!error.message!!.contains("cmd-1"))
            }
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `already-confirmed ineligible and conflict map distinctly`() = runTest {
        val server = MockWebServer()
        try {
            server.enqueue(
                MockResponse().setResponseCode(409)
                    .setBody("""{"code":"community.already-confirmed"}"""),
            )
            server.enqueue(
                MockResponse().setResponseCode(409)
                    .setBody("""{"code":"community.ineligible-target"}"""),
            )
            server.enqueue(
                MockResponse().setResponseCode(409)
                    .setBody("""{"code":"community.conflict"}"""),
            )
            val gateway = client(server)

            val first = try {
                gateway.submitConfirmation(observationId, "cmd-a")
                null
            } catch (error: CommunityVoteException) {
                error.kind
            }
            val second = try {
                gateway.submitDispute(observationId, "cmd-b", "OTHER", "private-note-x", null)
                null
            } catch (error: CommunityVoteException) {
                error.kind
            }
            val third = try {
                gateway.submitDispute(observationId, "cmd-c", "OTHER", null, null)
                null
            } catch (error: CommunityVoteException) {
                error.kind
            }

            assertEquals(CommunityVoteRejectKind.ALREADY_RECORDED, first)
            assertEquals(CommunityVoteRejectKind.INELIGIBLE_TARGET, second)
            assertEquals(CommunityVoteRejectKind.CONFLICT, third)
        } finally {
            server.shutdown()
        }
    }
}
