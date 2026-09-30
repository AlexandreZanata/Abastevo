package com.anpfuel.data.local.auth

import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.domain.portable.PortableAuth
import java.util.concurrent.TimeUnit
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class AccountHttpApiTest {

    private val server = MockWebServer()
    private val api = AccountHttpApi(
        OkHttpClient.Builder()
            .connectTimeout(2L, TimeUnit.SECONDS)
            .readTimeout(2L, TimeUnit.SECONDS)
            .build(),
        server.url("/").toString(),
    )

    private fun session() = PortableAuth.Session(
        familyId = "aaaaaaaa-1111-4111-8111-111111111111",
        accountId = "d6c74c23-63db-4c24-a2e5-408cb23bad26",
        accessToken = "tok-access", refreshToken = "tok-refresh",
        accessExpiresAt = 1_000_900L, absoluteExpiresAt = 4_259_200L,
    )

    private fun sessionJson() = """
        {"family_id":"aaaaaaaa-1111-4111-8111-111111111111",
         "account_id":"d6c74c23-63db-4c24-a2e5-408cb23bad26",
         "access_token":"tok-access","refresh_token":"tok-refresh",
         "access_expires_at":"2026-10-01T00:15:00Z",
         "absolute_expires_at":"2026-10-31T00:00:00Z"}
    """.trimIndent()

    @Test
    fun requestCodeAccepts() {
        server.enqueue(MockResponse().setResponseCode(202).setBody("""{"status":"sent"}"""))
        val res = api.requestCode("case-01@example.invalid")
        check(res is AuthApiResult.Ok)
        val recorded = server.takeRequest()
        assertTrue(recorded.path!!.endsWith("/v1/accounts/email/codes"))
    }

    @Test
    fun consumeParsesSessionAndCreated() {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"account":{"account_id":"d6c74c23-63db-4c24-a2e5-408cb23bad26",
                    "public_alias":"adore-fox-42","status":"active"},
                    "session":${sessionJson()},"created":true}""",
            ),
        )
        val res = api.consumeCode("case-01@example.invalid", "482916")
        check(res is AuthApiResult.Ok)
        assertTrue(res.value.created)
        assertEquals("tok-access", res.value.session.accessToken)
        assertTrue(res.value.session.accessExpiresAt > 0L)
        assertTrue(res.value.session.absoluteExpiresAt > res.value.session.accessExpiresAt)
    }

    @Test
    fun refreshParsesSession() {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"session":${sessionJson()}}"""))
        val res = api.refresh("family-1", "tok-refresh")
        check(res is AuthApiResult.Ok)
        assertEquals("tok-access", res.value.accessToken)
    }

    @Test
    fun revokeAndDeleteAccept() {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"status":"revoked"}"""))
        val revoke = api.revokeAll("family-1", "tok-access")
        check(revoke is AuthApiResult.Ok)
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"status":"deleted"}"""))
        val deleted = api.deleteAccount(session())
        check(deleted is AuthApiResult.Ok)
    }

    @Test
    fun linkAccepts() {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"provider_link":{"provider":"google","subject":"110229484794030244762",
                    "linked_at":"2026-10-01T00:00:00Z"}}""",
            ),
        )
        val res = api.linkProvider(session(), "google", "id-token", "n-07")
        check(res is AuthApiResult.Ok)
        val recorded = server.takeRequest()
        assertTrue(recorded.path!!.endsWith("/v1/accounts/providers/link"))
    }

    @Test
    fun errorEnvelopesMapVerbatim() {
        val cases = listOf(
            401 to "account.code-unknown" to "code-unknown",
            401 to "account.code-expired" to "code-expired",
            403 to "account.account-suspended" to "account-suspended",
            410 to "account.account-deleted" to "account-deleted",
            403 to "account.link-cross-account-refused" to "link-cross-account-refused",
            503 to "account.oidc-unavailable" to "oidc-unavailable",
        )
        for ((statusAndCode, want) in cases) {
            val (status, code) = statusAndCode
            server.enqueue(
                MockResponse().setResponseCode(status)
                    .setBody("""{"error":{"code":"$code","message":"x"}}"""),
            )
            val res = api.requestCode("case-01@example.invalid")
            check(res is AuthApiResult.Err)
            assertEquals(want, res.verdict)
        }
    }

    @Test
    fun transportAndProtocolFailuresAreUnavailable() {
        server.enqueue(MockResponse().setResponseCode(500).setBody("<html>down</html>"))
        val html = api.requestCode("case-01@example.invalid")
        check(html is AuthApiResult.Err)
        assertEquals(PortableAuth.UNAVAILABLE, html.verdict)
        server.enqueue(MockResponse().setResponseCode(200).setBody("not json"))
        val bad = api.consumeCode("case-01@example.invalid", "482916")
        check(bad is AuthApiResult.Err)
        assertEquals(PortableAuth.UNAVAILABLE, bad.verdict)
        val refused = AccountHttpApi(
            OkHttpClient.Builder()
                .connectTimeout(1L, TimeUnit.SECONDS)
                .readTimeout(1L, TimeUnit.SECONDS)
                .build(),
            "http://127.0.0.1:9/",
        )
        val down = refused.requestCode("case-01@example.invalid")
        check(down is AuthApiResult.Err)
        assertEquals(PortableAuth.UNAVAILABLE, down.verdict)
    }
}
