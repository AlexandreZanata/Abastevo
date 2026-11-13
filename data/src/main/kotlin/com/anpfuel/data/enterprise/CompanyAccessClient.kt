package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.enterprise.EnterprisePartition
import java.io.IOException
import java.time.Instant
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import okhttp3.Authenticator
import okhttp3.Call
import okhttp3.Callback
import okhttp3.Cookie
import okhttp3.CookieJar
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import org.json.JSONObject

/** Account cookie plane only. No credential, code, JSON, or channel state is persisted/logged. */
class CompanyAccessClient(
    http: OkHttpClient,
    origin: String,
    private val sessions: AuthSessionStore,
    private val auth: AuthFlow,
    private val fence: EnterpriseSessionFence,
) {
    private val origin = EnterprisePartition.canonicalOrigin(origin)
    private val url = this.origin.toHttpUrl()
    private val http = http.newBuilder().followRedirects(false).followSslRedirects(false)
        .retryOnConnectionFailure(false).cookieJar(CookieJar.NO_COOKIES).cache(null)
        .authenticator(Authenticator.NONE).proxyAuthenticator(Authenticator.NONE)
        .callTimeout(20, TimeUnit.SECONDS).build()
    private val monitor = Any()
    private var generation = 0L
    private var channel: Channel? = null
    private var retirement: Retirement? = null
    private val calls = mutableSetOf<Call>()

    private class Channel(val binding: EnterpriseAccessBinding, val cookie: String, val csrf: String) {
        override fun toString(): String = "CompanyChannel(redacted)"
    }

    /** This single memory-only hint can retire a prior cookie at login; it is never private authority. */
    private class Retirement(val account: String, val origin: String, val cookie: String) {
        fun matches(binding: EnterpriseAccessBinding?): Boolean = binding != null && binding.accountId == account && binding.origin == origin
        override fun toString(): String = "CompanyRetirement(redacted)"
    }

    fun currentBinding(): EnterpriseAccessBinding? = try {
        val before = fence.epoch
        val session = sessions.load()
        val after = fence.epoch
        if (before != after) null else session?.let { EnterpriseAccessBinding(it.accountId, origin, it.familyId, after) }
    } catch (_: Exception) { null }

    private class Reset(val token: Long, val cookie: String?, val calls: List<Call>)

    private fun resetLocked(current: EnterpriseAccessBinding?): Reset {
        channel?.takeIf { current != null && it.binding.accountId == current.accountId && it.binding.origin == current.origin }?.let {
            retirement = Retirement(it.binding.accountId, it.binding.origin, it.cookie)
        }
        if (retirement?.matches(current) != true) retirement = null
        generation++
        channel = null
        return Reset(generation, retirement?.cookie, calls.toList().also { calls.clear() })
    }

    fun invalidate() {
        val current = currentBinding()
        val reset = synchronized(monitor) { resetLocked(current) }
        reset.calls.forEach(Call::cancel)
    }

    suspend fun open(): EnterpriseAccessResult<EnterpriseAccessBinding> {
        val before = currentBinding() ?: run { invalidate(); return EnterpriseAccessResult.Err(EnterpriseAccessError.SIGN_IN_REQUIRED) }
        val background = fence.backgroundEpoch
        // Reset and capture one unique lease atomically; concurrent opens cannot adopt each other's generation.
        val reset = synchronized(monitor) { resetLocked(before) }
        reset.calls.forEach(Call::cancel)
        val token = reset.token
        val oldCookie = reset.cookie
        val refreshed = try { withContext(Dispatchers.IO) { auth.refreshSession() } }
        catch (cancelled: CancellationException) { throw cancelled }
        catch (_: Exception) { return EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE) }
        val session = when (refreshed) {
            is AuthApiResult.Err -> return EnterpriseAccessResult.Err(
                if (refreshed.verdict == "unavailable" || refreshed.verdict == AuthFlow.SECURE_STORAGE_UNAVAILABLE) EnterpriseAccessError.UNAVAILABLE
                else EnterpriseAccessError.SIGN_IN_REQUIRED,
            )
            is AuthApiResult.Ok -> refreshed.value ?: return EnterpriseAccessResult.Err(EnterpriseAccessError.SIGN_IN_REQUIRED)
        }
        val selected = currentBinding()
        // Even same-account re-login starts a new ceremony. Ordinary same-family rotation preserves it.
        if (selected == null || selected != before ||
            session.accountId != selected.accountId || session.familyId != selected.familyId ||
            sessions.load() != session || background != fence.backgroundEpoch ||
            synchronized(monitor) { token != generation }
        ) return EnterpriseAccessResult.Err(EnterpriseAccessError.STALE)
        val response = exchange("/v1/org-access/session/login", JSONObject().put("family_id", session.familyId).put("access_token", session.accessToken),
            null, token, selected, oldCookie)
        return synchronized(monitor) {
            if (token != generation || currentBinding() != selected || background != fence.backgroundEpoch || sessions.load() != session) {
                return@synchronized EnterpriseAccessResult.Err(EnterpriseAccessError.STALE)
            }
            when (response) {
                is EnterpriseAccessResult.Err -> response
                is EnterpriseAccessResult.Ok -> try {
                    val body = response.value.body
                    require(body.get("account_id") == selected.accountId)
                    val csrf = body.strictString("csrf_token")
                    require(Regex("abv_company_csrf_[0-9a-f]{64}").matches(csrf))
                    val authenticated = Instant.parse(body.strictString("authenticated_at"))
                    val expires = Instant.parse(body.strictString("expires_at"))
                    // The server's absolute lifetime starts at cookie creation, not personal authentication.
                    // Creation time is deliberately absent here; this client cannot reconstruct that limit.
                    require(expires.isAfter(authenticated))
                    val headers = response.value.cookies
                    require(headers.size == 1)
                    val raw = headers.single()
                    val cookie = Cookie.parse(url, raw) ?: throw IllegalArgumentException()
                    require(cookie.name == COOKIE && Regex("abv_company_[0-9a-f]{64}").matches(cookie.value) &&
                        cookie.hostOnly && cookie.domain == url.host && cookie.secure && cookie.httpOnly && cookie.path == "/" &&
                        cookie.expiresAt > authenticated.toEpochMilli() &&
                        raw.split(';').drop(1).any { it.trim().equals("SameSite=Strict", ignoreCase = true) })
                    channel = Channel(selected, "$COOKIE=${cookie.value}", csrf)
                    retirement = null
                    EnterpriseAccessResult.Ok(selected)
                } catch (_: Exception) { EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE) }
            }
        }
    }

    /** JSON remains inside data adapters. Future read-only context transport can reuse this channel. */
    internal suspend fun post(binding: EnterpriseAccessBinding, path: String, body: JSONObject): EnterpriseAccessResult<JSONObject> {
        val (selected, token) = synchronized(monitor) {
            channel?.takeIf { it.binding == binding && currentBinding() == binding }?.let { it to generation }
        } ?: return EnterpriseAccessResult.Err(EnterpriseAccessError.STALE)
        val response = exchange(path, body, selected, token, binding)
        return synchronized(monitor) {
            if (channel !== selected || currentBinding() != binding) {
                if (channel === selected) { generation++; channel = null }
                EnterpriseAccessResult.Err(EnterpriseAccessError.STALE)
            } else when (response) {
                is EnterpriseAccessResult.Err -> {
                    if (response.error in setOf(EnterpriseAccessError.SIGN_IN_REQUIRED, EnterpriseAccessError.FRESH_AUTH_REQUIRED)) {
                        retirement = Retirement(selected.binding.accountId, selected.binding.origin, selected.cookie)
                        generation++; channel = null
                    }
                    response
                }
                is EnterpriseAccessResult.Ok -> {
                    if (response.value.cookies.isNotEmpty()) EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE)
                    else EnterpriseAccessResult.Ok(response.value.body)
                }
            }
        }
    }

    private class Reply(val body: JSONObject, val cookies: List<String>) {
        override fun toString(): String = "CompanyReply(redacted)"
    }

    private suspend fun exchange(path: String, body: JSONObject, selected: Channel?, token: Long,
        binding: EnterpriseAccessBinding, oldCookie: String? = null): EnterpriseAccessResult<Reply> = withContext(Dispatchers.IO) {
        exchangeIo(path, body, selected, token, binding, oldCookie)
    }

    private suspend fun exchangeIo(path: String, body: JSONObject, selected: Channel?, token: Long,
        binding: EnterpriseAccessBinding, oldCookie: String?): EnterpriseAccessResult<Reply> {
        if (!path.startsWith("/v1/") || path.any { it == '?' || it == '#' } || path.contains("..") || body.toString().toByteArray().size > 16 * 1024) {
            return EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE)
        }
        var owned: Call? = null
        return try {
            val request = Request.Builder().url(origin + path).header("Origin", origin)
                .header("Accept", "application/json").header("Cache-Control", "no-store")
                .post(body.toString().toRequestBody(JSON)).apply {
                    if (selected != null) { header("Cookie", selected.cookie); header("X-CSRF", selected.csrf) }
                    // The login endpoint uses the proven account to retire this cookie; no private DTO can borrow it.
                    else if (path == "/v1/org-access/session/login" && oldCookie != null) header("Cookie", oldCookie)
                }.build()
            val call = http.newCall(request)
            val allowed = synchronized(monitor) {
                if (token != generation || currentBinding() != binding || (selected != null && channel !== selected)) false
                else { calls.add(call); true }
            }
            if (!allowed) return EnterpriseAccessResult.Err(EnterpriseAccessError.STALE)
            owned = call
            awaitCompanyResponse(call).use { response ->
                val content = response.body ?: return EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE)
                if (content.contentType()?.let { it.type == "application" && it.subtype == "json" } != true ||
                    !response.headers.values("Cache-Control").any { it.split(',').any { directive -> directive.trim().equals("no-store", true) } }
                ) return EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE)
                val source = content.source()
                if (source.request(BODY_LIMIT + 1)) return EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE)
                val parsed = JSONObject(source.readUtf8())
                if (response.code != 200) return EnterpriseAccessResult.Err(error(response.code, parsed))
                EnterpriseAccessResult.Ok(Reply(parsed, response.headers.values("Set-Cookie")))
            }
        } catch (cancelled: CancellationException) { throw cancelled }
        catch (_: Exception) { EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE) }
        finally { if (owned != null) synchronized(monitor) { calls.remove(owned) } }
    }

    private fun error(status: Int, body: JSONObject): EnterpriseAccessError = when (status to body.optJSONObject("error")?.optString("code")) {
        401 to "UNAUTHENTICATED" -> EnterpriseAccessError.SIGN_IN_REQUIRED
        401 to "FRESH_AUTH_REQUIRED" -> EnterpriseAccessError.FRESH_AUTH_REQUIRED
        404 to "TOKEN_INVALID" -> EnterpriseAccessError.INVALID_CODE
        409 to "CONFLICT" -> EnterpriseAccessError.CONFLICT
        409 to "ALREADY_MEMBER" -> EnterpriseAccessError.ALREADY_MEMBER
        403 to "MANAGED_ACCOUNT_RESTRICTED" -> EnterpriseAccessError.MANAGED_RESTRICTED
        429 to "RATE_LIMITED" -> EnterpriseAccessError.RATE_LIMITED
        else -> EnterpriseAccessError.UNAVAILABLE
    }

    override fun toString(): String = "CompanyAccessClient(redacted)"
    companion object {
        private const val COOKIE = "__Host-empresas-session"
        private const val BODY_LIMIT = 64L * 1024
        private val JSON = "application/json; charset=utf-8".toMediaType()
    }
}

/** Cancellable ownership transfer also closes a response rejected by prompt coroutine cancellation. */
internal suspend fun awaitCompanyResponse(call: Call): Response = suspendCancellableCoroutine { continuation ->
    continuation.invokeOnCancellation { call.cancel() }
    call.enqueue(object : Callback {
        override fun onFailure(call: Call, e: IOException) {
            if (!continuation.isCancelled) continuation.resumeWithException(IOException("Company request unavailable"))
        }
        override fun onResponse(call: Call, response: Response) {
            continuation.resume(response) { _, rejected, _ -> rejected.close() }
        }
    })
}

internal fun JSONObject.strictString(key: String): String = (get(key) as? String) ?: throw IllegalArgumentException("Invalid company response")
internal fun JSONObject.strictBoolean(key: String): Boolean = (get(key) as? Boolean) ?: throw IllegalArgumentException("Invalid company response")
