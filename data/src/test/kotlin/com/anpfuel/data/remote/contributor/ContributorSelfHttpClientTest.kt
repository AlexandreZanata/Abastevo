package com.anpfuel.data.remote.contributor

import com.anpfuel.domain.contributor.ContributorException
import com.anpfuel.domain.contributor.ProfileEdit
import com.anpfuel.domain.contributor.ProfileVisibility
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ContributorSelfHttpClientTest {

    private fun client(server: MockWebServer) =
        ContributorSelfHttpClient(OkHttpClient(), server.url("/").toString())

    private val claim = """
        {"claim_id":"11111111-1111-4111-8111-111111111111","github_login":"octocat",
         "state":"PENDING","proof_status":"UNVERIFIED","override":false,
         "created_at":"2026-10-27T12:00:00Z","expires_at":"2026-11-26T12:00:00Z"}
    """.trimIndent()

    @Test fun `open claim sends proof and parses code`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody("""{"claim_id":"11111111-1111-4111-8111-111111111111","code":"abastevo-contributor-AAAAAAAAAAAAAAAA"}"""))
            val issued = runBlocking {
                client(server).openClaim("fam", "token", "octocat", "22222222-2222-4222-8222-222222222222")
            }
            assertEquals("11111111-1111-4111-8111-111111111111", issued.claimId)
            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            val body = JSONObject(request.body.readUtf8())
            assertEquals("fam", body.getString("family_id"))
            assertEquals("token", body.getString("access_token"))
            assertEquals("octocat", body.getString("github_login"))
            assertEquals("22222222-2222-4222-8222-222222222222", body.getString("operation_id"))
        }
    }

    @Test fun `read claims keeps the pending code for re-display`() = runTest {
        MockWebServer().use { server ->
            val withCode = claim.replace("\"override\":false", "\"override\":false,\"code\":\"abastevo-contributor-BBBBBBBBBBBBBBBB\"")
            server.enqueue(MockResponse().setBody("""{"items":[$withCode,${claim.replace("PENDING", "APPROVED")}]}"""))
            val claims = runBlocking { client(server).readClaims("fam", "token") }
            assertEquals("abastevo-contributor-BBBBBBBBBBBBBBBB", claims[0].code)
            assertEquals(null, claims[1].code)
            assertTrue(server.takeRequest(1, TimeUnit.SECONDS)!!.path!!.endsWith("/claims/read"))
        }
    }

    @Test fun `proof and withdraw hit their routes`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody(claim.replace("UNVERIFIED", "VERIFIED")))
            server.enqueue(MockResponse().setBody(claim.replace("PENDING", "WITHDRAWN")))
            val client = client(server)
            val proven = runBlocking { client.submitProof("fam", "token", "11111111-1111-4111-8111-111111111111", "gist1") }
            assertEquals("VERIFIED", proven.proof.name)
            val withdrawn = runBlocking { client.withdrawClaim("fam", "token", "11111111-1111-4111-8111-111111111111") }
            assertEquals("WITHDRAWN", withdrawn.state.name)
            val proofRequest = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertTrue(proofRequest.path!!.endsWith("/proof"))
            assertTrue(JSONObject(proofRequest.body.readUtf8()).getString("gist_id") == "gist1")
            val withdrawRequest = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertTrue(withdrawRequest.path!!.endsWith("/withdraw"))
        }
    }

    @Test fun `profile upsert sends links and consent`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(
                MockResponse().setBody(
                    """{"profile":{"display_name":"Ana","links":[],"visibility":"PUBLIC","public_consent":true,"show_roles_publicly":false},"roles":[]}""",
                ),
            )
            val read = runBlocking {
                client(server).upsertProfile(
                    "fam",
                    "token",
                    ProfileEdit("Ana", emptyList(), ProfileVisibility.PUBLIC, true, false),
                )
            }
            assertEquals("Ana", read.profile!!.displayName)
            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            val body = JSONObject(request.body.readUtf8())
            assertEquals(true, body.getBoolean("public_consent"))
            assertEquals("PUBLIC", body.getString("visibility"))
        }
    }

    @Test fun `error codes pass through verbatim`() = runTest {
        val cases = listOf(
            401 to ContributorException.Denied::class.java,
            403 to ContributorException.Forbidden::class.java,
            409 to ContributorException.Conflict::class.java,
            429 to ContributorException.Quota::class.java,
        )
        for ((status, type) in cases) {
            MockWebServer().use { server ->
                server.enqueue(
                    MockResponse().setResponseCode(status)
                        .setBody("""{"error":{"code":"contributor.x","message":"x"}}"""),
                )
                val thrown = try {
                    runBlocking {
                        client(server).openClaim("fam", "token", "octocat", "22222222-2222-4222-8222-222222222222")
                    }
                    null
                } catch (e: Exception) {
                    e
                }
                assertTrue(type.isInstance(thrown), "$status -> $type, got $thrown")
            }
        }
    }

    @Test fun `conflict keeps the server code`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(
                MockResponse().setResponseCode(409)
                    .setBody("""{"error":{"code":"contributor.claim-state","message":"x"}}"""),
            )
            val thrown = try {
                runBlocking {
                    client(server).openClaim("fam", "token", "octocat", "22222222-2222-4222-8222-222222222222")
                }
                null
            } catch (e: ContributorException.Conflict) {
                e
            }
            assertEquals("contributor.claim-state", (thrown as ContributorException.Conflict).message)
        }
    }
}
