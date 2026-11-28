package com.anpfuel.data.remote.notifications

import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.data.local.notifications.NotificationJsonCodec
import com.anpfuel.data.remote.PhotoProofTransport
import com.anpfuel.domain.notification.AccountNotificationGateway
import com.anpfuel.domain.notification.AccountNotificationPage
import com.anpfuel.domain.portable.PortableAuth
import java.io.IOException
import java.util.UUID
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONArray
import org.json.JSONObject

/**
 * B-BR-NOTIF-03 transport. The list is a private session read (like the
 * ledger record pages); marking read is a write signed by the device key
 * bound to the account. No-store, no redirects, bounded bodies.
 */
class AccountNotificationsHttpClient(
    http: OkHttpClient,
    baseUrl: String,
    private val auth: AuthFlow,
    private val proof: PhotoProofTransport,
) : AccountNotificationGateway {

    private val origin = baseUrl.toHttpUrl()
    private val client = http.newBuilder().followRedirects(false).followSslRedirects(false)
        .retryOnConnectionFailure(false).callTimeout(20, TimeUnit.SECONDS).build()

    override suspend fun list(limit: Int, before: String?): AccountNotificationPage? {
        require(limit in 1..MAX_LIMIT)
        val session = liveSession() ?: return null
        val body = JSONObject().put("family_id", session.familyId).put("access_token", session.accessToken).put("limit", limit)
        if (before != null) body.put("before", before)
        val root = postSession(LIST_PATH, body)
        val array = root.optJSONArray("items") ?: throw IOException("notifications.invalid")
        if (array.length() > MAX_LIMIT) throw IOException("notifications.invalid")
        val items = (0 until array.length()).mapNotNull { index ->
            array.optJSONObject(index)?.let(NotificationJsonCodec::decodeServerItem)
        }
        return AccountNotificationPage(
            items = items,
            nextBefore = root.optString("next_before").takeIf { it.isNotBlank() && it != "null" },
            unread = root.optInt("unread", items.count { !it.read }).coerceAtLeast(0),
        )
    }

    override suspend fun markRead(ids: Set<String>) {
        if (ids.isEmpty()) return
        ids.chunked(MAX_LIMIT).forEach { chunk -> signedRead(JSONObject().put("notification_ids", JSONArray(chunk))) }
    }

    override suspend fun markAllRead() = signedRead(JSONObject().put("all", true))

    private suspend fun signedRead(selection: JSONObject) {
        val session = liveSession() ?: throw IOException("notifications.sign-in-required")
        val operation = UUID.randomUUID().toString()
        val scope = proof.localScope()
        if (!scope.startsWith("account:${session.accountId}:")) throw IOException("notifications.owner-changed")
        val doc = JSONObject(selection.toString())
            .put("family_id", session.familyId)
            .put("access_token", session.accessToken)
            .put("operation_id", operation)
        proof.post(READ_PATH, operation, doc, scope)
    }

    private suspend fun liveSession(): PortableAuth.Session? = withContext(Dispatchers.IO) {
        when (val result = auth.refreshSession()) {
            is AuthApiResult.Err -> throw IOException("notifications.session-unavailable")
            is AuthApiResult.Ok -> result.value?.takeIf { PortableAuth.isAccessLive(it, System.currentTimeMillis() / 1000) }
        }
    }

    private suspend fun postSession(path: String, body: JSONObject): JSONObject = withContext(Dispatchers.IO) {
        val url = origin.newBuilder().addPathSegments(path.removePrefix("/")).build()
        val request = Request.Builder().url(url)
            .header("Accept", "application/json")
            .header("Cache-Control", "no-store")
            .post(body.toString().toByteArray(Charsets.UTF_8).toRequestBody(JSON))
            .build()
        client.newCall(request).execute().use { response ->
            when (response.code) {
                200 -> Unit
                401, 403 -> throw IOException("notifications.denied")
                else -> throw IOException("notifications.unavailable")
            }
            val text = response.body?.source()?.let { source ->
                if (!source.request(MAX_BODY_BYTES + 1) || source.buffer.size <= MAX_BODY_BYTES) source.readUtf8() else null
            } ?: throw IOException("notifications.too-large")
            JSONObject(text)
        }
    }

    companion object {
        const val LIST_PATH = "/v1/notifications/list"
        const val READ_PATH = "/v1/notifications/read"
        private const val MAX_LIMIT = 50
        private const val MAX_BODY_BYTES = 256L * 1024
        private val JSON = "application/json; charset=utf-8".toMediaType()
    }
}
