package com.anpfuel.data.local.auth

import com.anpfuel.application.portable.AuthAccountApi
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.ConsumeOk
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
    private val client: OkHttpClient,
    private val baseUrl: String,
) : AuthAccountApi {

    override fun requestCode(email: String): AuthApiResult<Unit> {
        val res = post("/v1/accounts/email/codes", JSONObject().put("email", email))
        return when (res) {
            is HttpResult.Ok -> AuthApiResult.Ok(Unit)
            is HttpResult.Err -> AuthApiResult.Err(res.verdict)
        }
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
                val username = res.body.optJSONObject("account")?.optString("public_alias", "").orEmpty()
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
                it.body?.string() ?: ""
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
            PortableAuth.Session(
                familyId = doc.getString("family_id"),
                accountId = doc.getString("account_id"),
                accessToken = doc.getString("access_token"),
                refreshToken = doc.getString("refresh_token"),
                accessExpiresAt = epochOf(doc.getString("access_expires_at")),
                absoluteExpiresAt = epochOf(doc.getString("absolute_expires_at")),
            )
        } catch (_: Exception) {
            null
        }
    }

    private fun epochOf(rfc3339: String): Long {
        return OffsetDateTime.parse(rfc3339).toEpochSecond()
    }

    companion object {
        private val JSON = "application/json; charset=utf-8".toMediaType()
    }
}
