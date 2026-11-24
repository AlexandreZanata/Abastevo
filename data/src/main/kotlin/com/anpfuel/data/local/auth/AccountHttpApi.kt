package com.anpfuel.data.local.auth

import com.anpfuel.application.portable.AuthAccountApi
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.ConsumeOk
import com.anpfuel.application.portable.GoogleLogin
import com.anpfuel.application.portable.IssuedAccountKey
import com.anpfuel.application.portable.KeyLogin
import com.anpfuel.domain.portable.PortableAuth
import java.io.IOException
import java.time.OffsetDateTime
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

/**
 * OkHttp [AuthAccountApi] over the frozen backend account contract
 * (P13-T05B, P13-T02…T04).
 *
 * Blocking calls fit the synchronous portable flows; threading stays
 * with the caller (coroutines on device). Error envelopes
 * `{"error":{"code":"account.<verdict>"}}` map to their verdict
 * verbatim for UI mapping; transport failures, non-2xx surprises and
 * unparsable replies map to [PortableAuth.UNAVAILABLE] (retryable,
 * never an auth refusal). Times arrive RFC3339 and convert to epoch
 * seconds here, keeping portable code clock-pure.
 */
class AccountHttpApi(
    client: OkHttpClient,
    private val baseUrl: String,
) : AuthAccountApi {
    // Account keys and bearer tokens must never follow a redirect, even
    // when a caller supplies an otherwise permissive HTTP client.
    private val client = client.newBuilder()
        .followRedirects(false)
        .followSslRedirects(false)
        .callTimeout(20L, java.util.concurrent.TimeUnit.SECONDS)
        .build()

    override fun requestCode(email: String): AuthApiResult<Unit> {
        val res = post("/v1/accounts/email/codes", JSONObject().put("email", email))
        return when (res) {
            is HttpResult.Ok -> AuthApiResult.Ok(Unit)
            is HttpResult.Err -> AuthApiResult.Err(res.verdict)
        }
    }

    override fun requestCode(email: String, purpose: PortableAuth.CodePurpose, locale: String): AuthApiResult<Unit> =
        postParsed(
            "/v1/accounts/email/codes",
            JSONObject().put("email", email).put("purpose", purpose.wire).put("locale", locale),
        ) { Unit }

    override fun signUp(name: String, email: String, code: String): AuthApiResult<IssuedAccountKey> =
        postParsed(
            "/v1/accounts/keys/signup",
            emailProof(email, code).put("name", name),
        ) { parseIssuedKey(it, "created") }

    override fun recover(email: String, code: String): AuthApiResult<IssuedAccountKey> =
        postParsed("/v1/accounts/recovery", emailProof(email, code)) { parseIssuedKey(it, "sessions_revoked") }

    override fun loginWithGoogle(idToken: String, rawNonce: String): AuthApiResult<GoogleLogin> =
        postParsed("/v1/accounts/providers/login", googleProof(JSONObject(), idToken, rawNonce)) { body ->
            parseSession(body.getJSONObject("session"))?.let { session ->
                GoogleLogin(session, body.getBoolean("created"), body.getBoolean("profile_complete"))
            }
        }

    override fun profile(session: PortableAuth.Session): AuthApiResult<PortableAuth.Profile> =
        postParsed("/v1/accounts/profile", sessionProof(session)) { parseProfile(it, session.accountId) }

    override fun updateName(session: PortableAuth.Session, name: String): AuthApiResult<PortableAuth.Profile> =
        postParsed("/v1/accounts/profile/name", sessionProof(session).put("name", name)) {
            parseProfile(it, session.accountId)
        }

    override fun setRecoveryEmail(
        session: PortableAuth.Session,
        email: String,
        code: String,
    ): AuthApiResult<PortableAuth.Profile> =
        postParsed("/v1/accounts/recovery-email", sessionProof(session).put("email", email).put("code", code)) {
            parseProfile(it, session.accountId)
        }

    override fun linkGoogle(session: PortableAuth.Session, idToken: String, rawNonce: String): AuthApiResult<Unit> =
        postParsed("/v1/accounts/providers/link-native", googleProof(sessionProof(session), idToken, rawNonce)) { body ->
            Unit.takeIf { body.getJSONObject("provider_link").getString("provider") == PortableAuth.PROVIDER_GOOGLE }
        }

    override fun consumeCode(email: String, code: String): AuthApiResult<ConsumeOk> {
        val res = post(
            "/v1/accounts/email/consume",
            JSONObject().put("email", email).put("code", code),
        )
        return when (res) {
            is HttpResult.Err -> AuthApiResult.Err(res.verdict)
            is HttpResult.Ok -> {
                val body = res.body
                val session = body.optJSONObject("session")?.let(::parseSession)
                    ?: return AuthApiResult.Err(PortableAuth.UNAVAILABLE)
                AuthApiResult.Ok(ConsumeOk(session, body.optBoolean("created", false)))
            }
        }
    }

    override fun createKeyAccount(username: String): AuthApiResult<AuthFlow.KeyIssued> {
        val res = post("/v1/accounts/keys", JSONObject().put("username", username))
        return when (res) {
            is HttpResult.Err -> AuthApiResult.Err(res.verdict)
            is HttpResult.Ok -> {
                val name = res.body.optString("username", "")
                val key = res.body.optString("account_key", "")
                if (name.isEmpty() || key.isEmpty()) return AuthApiResult.Err(PortableAuth.UNAVAILABLE)
                AuthApiResult.Ok(AuthFlow.KeyIssued(name, key))
            }
        }
    }

    override fun loginWithKey(accountKey: String): AuthApiResult<KeyLogin> {
        val res = post("/v1/accounts/keys/login", JSONObject().put("account_key", accountKey))
        return when (res) {
            is HttpResult.Err -> AuthApiResult.Err(res.verdict)
            is HttpResult.Ok -> {
                val session = res.body.optJSONObject("session")?.let(::parseSession)
                    ?: return AuthApiResult.Err(PortableAuth.UNAVAILABLE)
                val account = res.body.optJSONObject("account")
                val username = account?.optString("public_name", "").orEmpty()
                    .ifEmpty { account?.optString("public_alias", "").orEmpty() }
                if (username.isEmpty()) return AuthApiResult.Err(PortableAuth.UNAVAILABLE)
                AuthApiResult.Ok(KeyLogin(session, username))
            }
        }
    }

    override fun refresh(familyId: String, refreshToken: String): AuthApiResult<PortableAuth.Session> {

        val res = post(
            "/v1/accounts/sessions/refresh",
            JSONObject().put("family_id", familyId).put("refresh_token", refreshToken),
        )
        return when (res) {
            is HttpResult.Err -> AuthApiResult.Err(res.verdict)
            is HttpResult.Ok -> {
                val session = res.body.optJSONObject("session")?.let(::parseSession)
                    ?: return AuthApiResult.Err(PortableAuth.UNAVAILABLE)
                AuthApiResult.Ok(session)
            }
        }
    }

    override fun revokeAll(familyId: String, accessToken: String): AuthApiResult<Unit> {
        val res = post(
            "/v1/accounts/sessions/revoke",
            JSONObject().put("family_id", familyId).put("access_token", accessToken),
        )
        return when (res) {
            is HttpResult.Ok -> AuthApiResult.Ok(Unit)
            is HttpResult.Err -> AuthApiResult.Err(res.verdict)
        }
    }

    override fun linkProvider(
        session: PortableAuth.Session,
        provider: String,
        idToken: String,
        nonce: String,
    ): AuthApiResult<Unit> {
        val res = post(
            "/v1/accounts/providers/link",
            JSONObject()
                .put("family_id", session.familyId)
                .put("access_token", session.accessToken)
                .put("provider", provider)
                .put("id_token", idToken)
                .put("nonce", nonce),
        )
        return when (res) {
            is HttpResult.Ok -> AuthApiResult.Ok(Unit)
            is HttpResult.Err -> AuthApiResult.Err(res.verdict)
        }
    }

    override fun deleteAccount(session: PortableAuth.Session): AuthApiResult<Unit> {
        val res = post(
            "/v1/accounts/deletion",
            JSONObject()
                .put("family_id", session.familyId)
                .put("access_token", session.accessToken),
        )
        return when (res) {
            is HttpResult.Ok -> AuthApiResult.Ok(Unit)
            is HttpResult.Err -> AuthApiResult.Err(res.verdict)
        }
    }

    private fun emailProof(email: String, code: String): JSONObject =
        JSONObject().put("email", email).put("code", code)

    private fun sessionProof(session: PortableAuth.Session): JSONObject =
        JSONObject().put("family_id", session.familyId).put("access_token", session.accessToken)

    private fun googleProof(body: JSONObject, idToken: String, nonce: String): JSONObject =
        body.put("provider", PortableAuth.PROVIDER_GOOGLE).put("id_token", idToken).put("nonce", nonce)

    private fun parseIssuedKey(body: JSONObject, flag: String): IssuedAccountKey? {
        if (!body.getBoolean(flag)) return null
        val session = parseSession(body.getJSONObject("session")) ?: return null
        val account = body.getJSONObject("account")
        if (session.accountId != account.getString("account_id")) return null
        val publicName = account.getString("public_name")
        val key = body.getString("account_key")
        if (publicName.isBlank() || key.isBlank()) return null
        return IssuedAccountKey(session, publicName, key)
    }

    private fun parseProfile(body: JSONObject, expectedAccountId: String): PortableAuth.Profile? {
        val accountId = body.getString("account_id")
        val publicName = body.getString("public_name")
        if (accountId != expectedAccountId || publicName.isBlank()) return null
        val providers = body.getJSONArray("providers")
        return PortableAuth.Profile(
            accountId,
            publicName,
            body.nullableString("name"),
            body.getBoolean("profile_complete"),
            body.nullableString("recovery_email_hint"),
            body.getBoolean("has_account_key"),
            (0 until providers.length()).map(providers::getString).toSet(),
        )
    }

    private fun JSONObject.nullableString(key: String): String? =
        if (isNull(key)) null else getString(key)

    private fun <T : Any> postParsed(path: String, payload: JSONObject, decode: (JSONObject) -> T?): AuthApiResult<T> =
        when (val result = post(path, payload)) {
            is HttpResult.Err -> AuthApiResult.Err(result.verdict)
            is HttpResult.Ok -> {
                val decoded = try {
                    decode(result.body)
                } catch (_: org.json.JSONException) {
                    null
                }
                decoded?.let { AuthApiResult.Ok(it) } ?: AuthApiResult.Err(PortableAuth.UNAVAILABLE)
            }
        }

    private sealed interface HttpResult {
        data class Ok(val body: JSONObject) : HttpResult
        data class Err(val verdict: String) : HttpResult
    }

    private fun post(path: String, body: JSONObject): HttpResult {
        val request = Request.Builder()
            .url(baseUrl.trimEnd('/') + path)
            .post(body.toString().toRequestBody(JSON))
            .header("Accept", "application/json")
            .build()
        val response = try {
            client.newCall(request).execute()
        } catch (_: IOException) {
            return HttpResult.Err(PortableAuth.UNAVAILABLE)
        } catch (_: Exception) {
            return HttpResult.Err(PortableAuth.UNAVAILABLE)
        }
        response.use {
            val raw = try {
                val body = it.body ?: return HttpResult.Err(PortableAuth.UNAVAILABLE)
                if (body.contentLength() > MAX_RESPONSE_BYTES) return HttpResult.Err(PortableAuth.UNAVAILABLE)
                val source = body.source()
                source.request(MAX_RESPONSE_BYTES + 1L)
                if (source.buffer.size > MAX_RESPONSE_BYTES) return HttpResult.Err(PortableAuth.UNAVAILABLE)
                source.readUtf8()
            } catch (_: Exception) {
                return HttpResult.Err(PortableAuth.UNAVAILABLE)
            }
            if (!it.isSuccessful) {
                return HttpResult.Err(parseVerdict(raw))
            }
            val doc = try {
                JSONObject(raw)
            } catch (_: Exception) {
                return HttpResult.Err(PortableAuth.UNAVAILABLE)
            }
            return HttpResult.Ok(doc)
        }
    }

    private fun parseVerdict(raw: String): String {
        return try {
            val code = JSONObject(raw)
                .optJSONObject("error")
                ?.optString("code", "")
                .orEmpty()
            if (code.startsWith("account.")) code.removePrefix("account.") else PortableAuth.UNAVAILABLE
        } catch (_: Exception) {
            PortableAuth.UNAVAILABLE
        }
    }

    private fun parseSession(doc: JSONObject): PortableAuth.Session? {
        return try {
            val session = PortableAuth.Session(
                familyId = doc.getString("family_id"),
                accountId = doc.getString("account_id"),
                accessToken = doc.getString("access_token"),
                refreshToken = doc.getString("refresh_token"),
                accessExpiresAt = epochOf(doc.getString("access_expires_at")),
                absoluteExpiresAt = epochOf(doc.getString("absolute_expires_at")),
            )
            if (session.familyId.isBlank() || session.accountId.isBlank() ||
                session.accessToken.isBlank() || session.refreshToken.isBlank() ||
                session.accessExpiresAt <= 0L || session.absoluteExpiresAt <= session.accessExpiresAt
            ) null else session
        } catch (_: Exception) {
            null
        }
    }

    private fun epochOf(rfc3339: String): Long {
        return OffsetDateTime.parse(rfc3339).toEpochSecond()
    }

    companion object {
        private const val MAX_RESPONSE_BYTES = 64L * 1024L
        private val JSON = "application/json; charset=utf-8".toMediaType()
    }
}
