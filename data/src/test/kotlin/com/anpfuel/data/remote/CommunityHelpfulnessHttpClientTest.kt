package com.anpfuel.data.remote

import com.anpfuel.domain.repository.CommunityHelpfulnessException
import com.anpfuel.domain.repository.CommunityHelpfulnessRejectKind
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Assertions.fail
import org.junit.jupiter.api.Test

/**
 * CS08A: helpfulness HTTP mapping (B-BR-CS01).
 *
 * Exact tallies parse; frozen backend codes surface as kinds; unknown
 * statuses and transports never claim a vote; request paths and
 * submission ids travel verbatim for server-side convergence.
 */
class CommunityHelpfulnessHttpClientTest {

    private val server = MockWebServer()

    @AfterEach
    fun tearDown() {
        server.shutdown()
    }

    private fun client(): CommunityHelpfulnessHttpClient =
        CommunityHelpfulnessHttpClient(OkHttpClient(), server.url("/").toString())

    @Test
    fun `cast parses exact tally and overlay`() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(201).setBody(
                """{"vote_id":"v1","observation_id":"obs","decision_sequence":2,"helpful":1,"not_helpful":0,"total":1,"helpful_basis_points":10000,"my_choice":"HELPFUL"}""",
            ),
        )
        val receipt = client().castHelpfulness("obs", "sub-1", "HELPFUL", 2)
        assertEquals("v1", receipt.voteId)
        assertEquals(1L, receipt.helpful)
        assertEquals(10000L, receipt.helpfulBasisPoints)
        assertEquals("HELPFUL", receipt.myChoice)
        val request = server.takeRequest()
        assertEquals("/v1/observations/obs/helpfulness", request.path)
        assertTrue(request.body.readUtf8().contains("\"client_submission_id\":\"sub-1\""))
    }

    @Test
    fun `empty tally omits percentage`() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"observation_id":"obs","decision_sequence":2,"helpful":0,"not_helpful":0,"total":0}""",
            ),
        )
        val tally = client().tallyFor("obs", 2)
        assertEquals(0L, tally.helpful)
        assertNull(tally.helpfulBasisPoints)
        assertNull(tally.myChoice)
    }

    @Test
    fun `backend codes map to kinds`() = runTest {
        val cases = listOf(
            Triple(403, """{"code":"community.self-vote"}""", CommunityHelpfulnessRejectKind.SELF_VOTE),
            Triple(409, """{"code":"community.stale-revision"}""", CommunityHelpfulnessRejectKind.STALE_REVISION),
            Triple(409, """{"code":"community.ineligible-target"}""", CommunityHelpfulnessRejectKind.INELIGIBLE_TARGET),
            Triple(404, """{"code":"community.not-found"}""", CommunityHelpfulnessRejectKind.INELIGIBLE_TARGET),
            Triple(429, """{"code":"community.quota-exceeded"}""", CommunityHelpfulnessRejectKind.QUOTA_EXCEEDED),
            Triple(400, """{"code":"community.invalid-vote"}""", CommunityHelpfulnessRejectKind.INVALID_VOTE),
            Triple(500, """oops""", CommunityHelpfulnessRejectKind.TRANSPORT),
        )
        for ((code, body, kind) in cases) {
            server.enqueue(MockResponse().setResponseCode(code).setBody(body))
            try {
                client().castHelpfulness("obs", "sub", "HELPFUL", 2)
                fail("code $code must throw")
            } catch (error: CommunityHelpfulnessException) {
                assertEquals(kind, error.kind, "code $code")
            }
        }
    }

    @Test
    fun `remove posts sequence only`() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"observation_id":"obs","decision_sequence":2,"helpful":0,"not_helpful":0,"total":0}""",
            ),
        )
        client().removeHelpfulness("obs", 2)
        val request = server.takeRequest()
        assertEquals("/v1/observations/obs/helpfulness/remove", request.path)
        assertTrue(request.body.readUtf8().contains("\"decision_sequence\":2"))
    }
}
