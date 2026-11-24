package com.anpfuel.data.local.auth

import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.domain.portable.PortableAuth
import java.util.concurrent.TimeUnit
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.json.JSONObject
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
         "access_expires_at":"1970-01-12T14:01:40Z",
         "absolute_expires_at":"1970-02-19T07:06:40Z"}
    """.trimIndent()

    @Test
    fun codePurposeAndLocaleAreExplicit() {
        server.enqueue(MockResponse().setResponseCode(202).setBody("""{"status":"sent"}"""))
        val response = api.requestCode("test@example.invalid", PortableAuth.CodePurpose.RECOVERY, "pt-BR")
        assertTrue(response is AuthApiResult.Ok)
        val request = server.takeRequest()
        assertEquals("/v1/accounts/email/codes", request.path)
        val json = JSONObject(request.body.readUtf8())
        assertEquals("recovery", json.getString("purpose"))
        assertEquals("pt-BR", json.getString("locale"))
    }

    @Test
    fun signupAndRecoveryParseTheNewKeyAndSession() {
        for (recover in listOf(false, true)) {
            val resultFlag = if (recover) "sessions_revoked" else "created"
            server.enqueue(
                MockResponse().setResponseCode(if (recover) 200 else 201).setBody(
                    """{"account":{"account_id":"${session().accountId}","public_name":"Test Person"},
                        "session":${sessionJson()},"account_key":"synthetic-key","$resultFlag":true}""",
                ),
            )
            val email = "test@example.invalid"
            val result = if (recover) api.recover(email, "012345") else api.signUp("Test Person", email, "012345")
            check(result is AuthApiResult.Ok)
            assertEquals("synthetic-key", result.value.accountKey)
            assertEquals("Test Person", result.value.publicName)
            assertEquals(session().accountId, result.value.session.accountId)
            val request = server.takeRequest()
            assertEquals(if (recover) "/v1/accounts/recovery" else "/v1/accounts/keys/signup", request.path)
            val json = JSONObject(request.body.readUtf8())
            assertEquals(!recover, json.has("name"))
            assertEquals("012345", json.getString("code"))
        }
    }

    @Test
    fun nativeGoogleProofAndProfileUseAccountApiShapes() {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"session":${sessionJson()},"created":true,"profile_complete":false}""",
            ),
        )
        val google = api.loginWithGoogle("synthetic-token", "synthetic-raw-nonce")
        check(google is AuthApiResult.Ok)
        assertEquals(false, google.value.profileComplete)
        val googleRequest = server.takeRequest()
        assertEquals("/v1/accounts/providers/login", googleRequest.path)
        val googleBody = JSONObject(googleRequest.body.readUtf8())
        assertEquals("google", googleBody.getString("provider"))
        assertEquals("synthetic-raw-nonce", googleBody.getString("nonce"))

        val session = session()
        val profile = """{"account_id":"${session.accountId}","public_name":"synthetic-alias","name":null,
                         "profile_complete":false,"recovery_email_hint":null,"has_account_key":false,
                         "providers":["google"]}"""
        for ((route, call) in listOf(
            "profile" to { api.profile(session) },
            "profile/name" to { api.updateName(session, "Example Person") },
            "recovery-email" to { api.setRecoveryEmail(session, "test@example.invalid", "012345") },
        )) {
            server.enqueue(MockResponse().setResponseCode(200).setBody(profile))
            val result = call()
            check(result is AuthApiResult.Ok)
            assertEquals(setOf("google"), result.value.providers)
            assertEquals(null, result.value.name)
            assertEquals(null, result.value.recoveryEmailHint)
            val request = server.takeRequest()
            assertEquals("/v1/accounts/$route", request.path)
            val payload = JSONObject(request.body.readUtf8())
            assertEquals(session.familyId, payload.getString("family_id"))
            assertEquals(session.accessToken, payload.getString("access_token"))
        }

        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"provider_link":{"provider":"google"}}"""))
        assertTrue(api.linkGoogle(session, "synthetic-token", "synthetic-raw-nonce") is AuthApiResult.Ok)
        val link = server.takeRequest()
        assertEquals("/v1/accounts/providers/link-native", link.path)
        assertEquals(session.familyId, JSONObject(link.body.readUtf8()).getString("family_id"))
    }

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

    @Test
    fun createKeyAccountParsesIssuedKey() {
        server.enqueue(
            MockResponse().setResponseCode(201)
                .setBody("""{"username":"ana123","account_key":"key-for-ana123"}"""),
        )
        val res = api.createKeyAccount("ana123")
        check(res is AuthApiResult.Ok)
        assertEquals("ana123", res.value.username)
        assertEquals("key-for-ana123", res.value.accountKey)
    }

    @Test
    fun loginWithKeyParsesSessionAndUsername() {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"account":{"account_id":"d6c74c23-63db-4c24-a2e5-408cb23bad26",
                    "public_alias":"ana123","status":"active"},
                    "session":${sessionJson()},"created":false}""",
            ),
        )
        val res = api.loginWithKey("key-for-ana123")
        check(res is AuthApiResult.Ok)
        assertEquals(session(), res.value.session)
        assertEquals("ana123", res.value.username)
    }

    @Test
    fun keyVerdictsPassThroughVerbatim() {
        server.enqueue(
            MockResponse().setResponseCode(409)
                .setBody("""{"error":{"code":"account.username-taken","message":"x"}}"""),
        )
        val taken = api.createKeyAccount("ana123")
        check(taken is AuthApiResult.Err)
        assertEquals("username-taken", taken.verdict)
        server.enqueue(
            MockResponse().setResponseCode(401)
                .setBody("""{"error":{"code":"account.key-invalid","message":"x"}}"""),
        )
        val refused = api.loginWithKey("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
        check(refused is AuthApiResult.Err)
        assertEquals("key-invalid", refused.verdict)
    }
    @Test
    fun accountKeyNeverFollowsRedirectToAnotherDestination() {
        val destination = MockWebServer()
        try {
            destination.start()
            server.enqueue(MockResponse().setResponseCode(307).setHeader("Location", destination.url("/capture")))
            destination.enqueue(MockResponse().setResponseCode(200).setBody("{}"))
            val result = api.loginWithKey("synthetic-account-key")
            assertEquals(AuthApiResult.Err(PortableAuth.UNAVAILABLE), result)
            assertEquals(0, destination.requestCount)
        } finally {
            destination.shutdown()
        }
    }

    @Test
    fun oversizedResponseFailsClosed() {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"session":${sessionJson()},"padding":"${"x".repeat(70_000)}"}"""))
        assertEquals(AuthApiResult.Err(PortableAuth.UNAVAILABLE), api.refresh("family-1", "synthetic-refresh"))
    }

    @Test
    fun emptyTokensAndInvertedSessionLifetimesRefuse() {
        for (invalid in listOf(
            sessionJson().replace("tok-access", ""),
            sessionJson().replace("1970-02-19T07:06:40Z", "1970-01-01T00:00:00Z"),
        )) {
            server.enqueue(MockResponse().setResponseCode(200).setBody("""{"session":$invalid}"""))
            assertEquals(AuthApiResult.Err(PortableAuth.UNAVAILABLE), api.refresh("family-1", "synthetic-refresh"))
        }
    }

}
