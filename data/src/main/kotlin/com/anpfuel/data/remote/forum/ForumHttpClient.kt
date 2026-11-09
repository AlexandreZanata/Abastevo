package com.anpfuel.data.remote.forum

import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.mapper.ForumMapper
import com.anpfuel.domain.forum.ForumCategoriesResult
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumCommentReceipt
import com.anpfuel.domain.forum.ForumCommentsPage
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumFeedFilters
import com.anpfuel.domain.forum.ForumFeedPage
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.forum.ForumPostKind
import com.anpfuel.domain.forum.ForumRegion
import com.anpfuel.domain.forum.ForumRepliesPage
import com.anpfuel.domain.forum.ForumReportReceipt
import com.anpfuel.domain.forum.ForumTopicReceipt
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.repository.ForumGateway
import java.io.IOException
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import okhttp3.Call
import okhttp3.Callback
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import org.json.JSONObject

private const val BODY_BUDGET = 512L * 1024L + 1 // 512 KiB byte cap
private val SIGNED_JSON_MEDIA_TYPE = "application/json".toMediaType()

/**
 * HTTP transport client for Forum Abastevo (FOR-07, FOR-17, FOR-18, FOR-19).
 *
 * Anonymous reads for public feed/detail/comments/replies.
 * Session-backed authenticated commands for create/edit/remove/read/report.
 * Bounded by strict byte budget (512 KiB) and 15s timeout.
 */
class ForumHttpClient(
    http: OkHttpClient,
    baseUrl: String,
    private val sessions: AuthSessionStore? = null,
    private val auth: AuthFlow? = null,
    private val nowEpochSeconds: () -> Long = { System.currentTimeMillis() / 1000L },
    private val proofHeaders: (suspend (path: String, body: ByteArray) -> Map<String, String>)? = null,
) : ForumGateway {

    private val origin = baseUrl.trimEnd('/').toHttpUrl()
    private val client = http.newBuilder()
        .followRedirects(false)
        .followSslRedirects(false)
        .retryOnConnectionFailure(false)
        .callTimeout(15, TimeUnit.SECONDS)
        .build()

    private data class SessionCredentials(val familyId: String, val accessToken: String)

    private fun liveSession(): SessionCredentials {
        val store = sessions ?: throw ForumException.AuthRequired("forum.auth-required")
        val stored = try {
            if (auth == null) store.load() else when (val result = auth.refreshSession()) {
                is AuthApiResult.Ok -> result.value
                is AuthApiResult.Err -> null
            }
        } catch (_: Exception) {
            null
        }
        if (stored == null || !PortableAuth.isAccessLive(stored, nowEpochSeconds())) {
            throw ForumException.AuthRequired("forum.auth-required")
        }
        return SessionCredentials(stored.familyId, stored.accessToken)
    }

    override suspend fun getCategories(): ForumCategoriesResult {
        val url = origin.newBuilder()
            .addPathSegments("v1/forum/categories")
            .build()
        val raw = execute(Request.Builder().url(url).header("Accept", "application/json").build())
        return try {
            ForumMapper.categories(JSONObject(raw))
        } catch (e: ForumException) {
            throw e
        } catch (e: Exception) {
            throw ForumException.Invalid("forum.bad-response")
        }
    }

    override suspend fun getFeed(
        category: ForumCategory?,
        kind: ForumPostKind?,
        limit: Int,
        cursor: String?,
        filters: ForumFeedFilters,
    ): ForumFeedPage {
        val url = origin.newBuilder()
            .addPathSegments("v1/forum/posts")
            .apply {
                addQueryParameter("limit", limit.toString())
                if (category != null && category != ForumCategory.UNKNOWN) {
                    addQueryParameter("category", category.wireCode)
                }
                if (kind != null && kind != ForumPostKind.UNKNOWN) {
                    addQueryParameter("kind", kind.wireValue)
                }
                if (!cursor.isNullOrBlank()) {
                    addQueryParameter("cursor", cursor)
                }
                filters.uf?.takeIf { it.isNotBlank() }?.let { addQueryParameter("uf", it) }
                filters.municipalityIbge?.let { addQueryParameter("municipality_ibge", it.toString()) }
                filters.fuelProduct?.takeIf { it.isNotBlank() }?.let { addQueryParameter("fuel_product", it) }
                filters.stationId?.takeIf { it.isNotBlank() }?.let { addQueryParameter("station_id", it) }
            }
            .build()
        val raw = execute(Request.Builder().url(url).header("Accept", "application/json").build())
        return try {
            ForumMapper.feed(JSONObject(raw))
        } catch (e: ForumException) {
            throw e
        } catch (e: Exception) {
            throw ForumException.Invalid("forum.bad-response")
        }
    }

    override suspend fun getPostDetail(postId: String): ForumPostDetail {
        val url = origin.newBuilder()
            .addPathSegments("v1/forum/posts/$postId")
            .build()
        val raw = execute(Request.Builder().url(url).header("Accept", "application/json").build())
        return try {
            // The API wraps the detail: {"post": {...}}.
            ForumMapper.detail(JSONObject(raw).getJSONObject("post"))
        } catch (e: ForumException) {
            throw e
        } catch (e: Exception) {
            throw ForumException.Invalid("forum.bad-response")
        }
    }

    override suspend fun getComments(postId: String, limit: Int, cursor: String?): ForumCommentsPage {
        val url = origin.newBuilder()
            .addPathSegments("v1/forum/posts/$postId/comments")
            .apply {
                addQueryParameter("limit", limit.toString())
                if (!cursor.isNullOrBlank()) {
                    addQueryParameter("cursor", cursor)
                }
            }
            .build()
        val raw = execute(Request.Builder().url(url).header("Accept", "application/json").build())
        return try {
            ForumMapper.comments(JSONObject(raw))
        } catch (e: ForumException) {
            throw e
        } catch (e: Exception) {
            throw ForumException.Invalid("forum.bad-response")
        }
    }

    override suspend fun getReplies(commentId: String, limit: Int, cursor: String?): ForumRepliesPage {
        val url = origin.newBuilder()
            .addPathSegments("v1/forum/comments/$commentId/replies")
            .apply {
                addQueryParameter("limit", limit.toString())
                if (!cursor.isNullOrBlank()) {
                    addQueryParameter("cursor", cursor)
                }
            }
            .build()
        val raw = execute(Request.Builder().url(url).header("Accept", "application/json").build())
        return try {
            ForumMapper.replies(JSONObject(raw))
        } catch (e: ForumException) {
            throw e
        } catch (e: Exception) {
            throw ForumException.Invalid("forum.bad-response")
        }
    }

    override suspend fun createComment(
        postId: String,
        parentId: String?,
        bodyText: String,
        operationId: String,
    ): ForumCommentReceipt {
        val session = liveSession()
        val payload = JSONObject().apply {
            put("family_id", session.familyId)
            put("access_token", session.accessToken)
            put("operation_id", operationId)
            if (!parentId.isNullOrBlank()) {
                put("parent_id", parentId)
            }
            put("body_text", bodyText)
        }
        val raw = sessionPost("v1/forum/posts/$postId/comments", payload)
        return try {
            ForumMapper.commentReceipt(JSONObject(raw))
        } catch (e: ForumException) {
            throw e
        } catch (e: Exception) {
            throw ForumException.Invalid("forum.bad-response")
        }
    }

    override suspend fun editComment(
        commentId: String,
        expectedRevision: Long,
        bodyText: String,
        operationId: String,
    ): ForumCommentReceipt {
        val session = liveSession()
        val payload = JSONObject().apply {
            put("family_id", session.familyId)
            put("access_token", session.accessToken)
            put("operation_id", operationId)
            put("expected_revision", expectedRevision)
            put("body_text", bodyText)
        }
        val raw = sessionPost("v1/forum/comments/$commentId/edit", payload)
        return try {
            ForumMapper.commentReceipt(JSONObject(raw))
        } catch (e: ForumException) {
            throw e
        } catch (e: Exception) {
            throw ForumException.Invalid("forum.bad-response")
        }
    }

    override suspend fun removeComment(
        commentId: String,
        expectedRevision: Long,
        operationId: String,
    ): ForumCommentReceipt {
        val session = liveSession()
        val payload = JSONObject().apply {
            put("family_id", session.familyId)
            put("access_token", session.accessToken)
            put("operation_id", operationId)
            put("expected_revision", expectedRevision)
        }
        val raw = sessionPost("v1/forum/comments/$commentId/remove", payload)
        return try {
            ForumMapper.commentReceipt(JSONObject(raw))
        } catch (e: ForumException) {
            throw e
        } catch (e: Exception) {
            throw ForumException.Invalid("forum.bad-response")
        }
    }

    override suspend fun getOwnerComment(commentId: String): ForumCommentReceipt {
        val session = liveSession()
        val payload = JSONObject().apply {
            put("family_id", session.familyId)
            put("access_token", session.accessToken)
            put("comment_id", commentId)
        }
        val raw = sessionPost("v1/forum/owner/read", payload)
        return try {
            ForumMapper.commentReceipt(JSONObject(raw))
        } catch (e: ForumException) {
            throw e
        } catch (e: Exception) {
            throw ForumException.Invalid("forum.bad-response")
        }
    }

    override suspend fun report(
        targetType: String,
        targetId: String,
        reason: String,
        detail: String?,
    ): ForumReportReceipt {
        val session = liveSession()
        val payload = JSONObject().apply {
            put("family_id", session.familyId)
            put("access_token", session.accessToken)
            put("reason", reason)
            if (!detail.isNullOrBlank()) {
                put("detail", detail)
            }
        }
        val path = if (targetType == "FORUM_POST") {
            "v1/forum/posts/$targetId/report"
        } else {
            "v1/forum/comments/$targetId/report"
        }
        val raw = sessionPost(path, payload)
        return try {
            ForumMapper.reportReceipt(JSONObject(raw))
        } catch (e: ForumException) {
            throw e
        } catch (e: Exception) {
            throw ForumException.Invalid("forum.bad-response")
        }
    }

    override suspend fun createTopic(
        title: String,
        bodyText: String,
        category: ForumCategory,
        operationId: String,
        region: ForumRegion?,
        fuelProduct: String?,
        stationId: String?,
        publicAlias: String?,
    ): ForumTopicReceipt {
        val session = liveSession()
        val payload = JSONObject().apply {
            put("family_id", session.familyId)
            put("access_token", session.accessToken)
            put("operation_id", operationId)
            put("category", category.wireCode)
            put("title", title)
            put("body_text", bodyText)
            if (region != null && (!region.uf.isNullOrBlank() || region.municipalityIbge != null)) {
                put("region", JSONObject().apply {
                    if (!region.uf.isNullOrBlank()) put("uf", region.uf)
                    if (region.municipalityIbge != null) put("municipality_ibge", region.municipalityIbge)
                })
            }
            if (!fuelProduct.isNullOrBlank()) {
                put("fuel_product", fuelProduct)
            }
            if (!stationId.isNullOrBlank()) {
                put("station_id", stationId)
            }
            if (!publicAlias.isNullOrBlank()) {
                put("public_alias", publicAlias)
            }
        }
        val raw = sessionPost("v1/forum/topics", payload)
        return try {
            ForumMapper.topicReceipt(JSONObject(raw))
        } catch (e: ForumException) {
            throw e
        } catch (e: Exception) {
            throw ForumException.Invalid("forum.bad-response")
        }
    }

    override suspend fun editTopic(
        topicId: String,
        expectedRevision: Long,
        title: String,
        bodyText: String,
        category: ForumCategory,
        operationId: String,
        region: ForumRegion?,
        fuelProduct: String?,
        stationId: String?,
        publicAlias: String?,
    ): ForumTopicReceipt {
        val session = liveSession()
        val payload = JSONObject().apply {
            put("family_id", session.familyId)
            put("access_token", session.accessToken)
            put("operation_id", operationId)
            put("expected_revision", expectedRevision)
            put("category", category.wireCode)
            put("title", title)
            put("body_text", bodyText)
            if (region != null && (!region.uf.isNullOrBlank() || region.municipalityIbge != null)) {
                put("region", JSONObject().apply {
                    if (!region.uf.isNullOrBlank()) put("uf", region.uf)
                    if (region.municipalityIbge != null) put("municipality_ibge", region.municipalityIbge)
                })
            }
            if (!fuelProduct.isNullOrBlank()) {
                put("fuel_product", fuelProduct)
            }
            if (!stationId.isNullOrBlank()) {
                put("station_id", stationId)
            }
            if (!publicAlias.isNullOrBlank()) {
                put("public_alias", publicAlias)
            }
        }
        val raw = sessionPost("v1/forum/topics/$topicId/edit", payload)
        return try {
            ForumMapper.topicReceipt(JSONObject(raw))
        } catch (e: ForumException) {
            throw e
        } catch (e: Exception) {
            throw ForumException.Invalid("forum.bad-response")
        }
    }

    override suspend fun removeTopic(
        topicId: String,
        operationId: String,
    ): ForumTopicReceipt {
        val session = liveSession()
        val payload = JSONObject().apply {
            put("family_id", session.familyId)
            put("access_token", session.accessToken)
            put("operation_id", operationId)
        }
        val raw = sessionPost("v1/forum/topics/$topicId/remove", payload)
        return try {
            ForumMapper.topicReceipt(JSONObject(raw))
        } catch (e: ForumException) {
            throw e
        } catch (e: Exception) {
            throw ForumException.Invalid("forum.bad-response")
        }
    }

    override suspend fun getOwnCommentIds(postId: String): Set<String> {
        val session = liveSession()
        val payload = JSONObject().apply {
            put("family_id", session.familyId)
            put("access_token", session.accessToken)
            put("post_id", postId)
        }
        val raw = sessionPost("v1/forum/owner/read", payload)
        return try {
            ForumMapper.ownCommentIds(JSONObject(raw))
        } catch (e: ForumException) {
            throw e
        } catch (e: Exception) {
            throw ForumException.Invalid("forum.bad-response")
        }
    }

    override suspend fun getOwnerTopic(topicId: String): ForumTopicReceipt {
        val session = liveSession()
        val payload = JSONObject().apply {
            put("family_id", session.familyId)
            put("access_token", session.accessToken)
            put("topic_id", topicId)
        }
        val raw = sessionPost("v1/forum/owner/read", payload)
        return try {
            ForumMapper.topicReceipt(JSONObject(raw))
        } catch (e: ForumException) {
            throw e
        } catch (e: Exception) {
            throw ForumException.Invalid("forum.bad-response")
        }
    }

    /**
     * Session-backed command: the server binds the account session to the
     * device signing key, so the exact body bytes carry a request proof.
     */
    private suspend fun sessionPost(path: String, payload: JSONObject): String {
        val url = origin.newBuilder().addPathSegments(path).build()
        val body = payload.toString().toByteArray(Charsets.UTF_8)
        val proof = proofHeaders?.let { sign ->
            try {
                sign(url.encodedPath, body)
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                throw ForumException.Unavailable("forum.signing-unavailable")
            }
        }.orEmpty()
        val builder = Request.Builder()
            .url(url)
            .post(body.toRequestBody(SIGNED_JSON_MEDIA_TYPE))
            .header("Accept", "application/json")
            .header("Cache-Control", "no-store")
        proof.forEach { (name, value) -> builder.header(name, value) }
        return execute(builder.build())
    }

    private suspend fun execute(request: Request): String {
        return try {
            withContext(Dispatchers.IO) {
                suspendCancellableCoroutine { continuation ->
                    val call = client.newCall(request)
                    continuation.invokeOnCancellation { call.cancel() }
                    call.enqueue(object : Callback {
                        override fun onFailure(call: Call, e: IOException) {
                            if (continuation.isActive) {
                                continuation.resumeWithException(ForumException.Offline("forum.offline"))
                            }
                        }

                        override fun onResponse(call: Call, response: Response) {
                            response.use {
                                if (!continuation.isActive) return
                                try {
                                    val source = response.body?.source()
                                        ?: throw ForumException.Unavailable("forum.empty")
                                    if (source.request(BODY_BUDGET)) {
                                        throw ForumException.Invalid("forum.over-budget")
                                    }
                                    val body = source.readUtf8()

                                    when (response.code) {
                                        200, 201 -> continuation.resume(body)
                                        401 -> throw ForumException.AuthRequired("forum.auth-required")
                                        404 -> {
                                            val errCode = parseErrorCode(body)
                                            if (errCode == "forum.disabled") {
                                                throw ForumException.Disabled("forum.disabled")
                                            } else {
                                                throw ForumException.NotFound("forum.not-found")
                                            }
                                        }
                                        409 -> throw ForumException.Conflict("forum.conflict")
                                        429 -> {
                                            val retryHeader = response.header("Retry-After")?.toLongOrNull()
                                            val errCode = parseErrorCode(body) ?: "forum.rate-limited"
                                            throw ForumException.RateLimited(errCode, retryHeader)
                                        }
                                        400 -> {
                                            val errCode = parseErrorCode(body) ?: "forum.invalid"
                                            throw ForumException.Invalid(errCode)
                                        }
                                        else -> throw ForumException.Unavailable("forum.unavailable")
                                    }
                                } catch (e: Exception) {
                                    if (continuation.isActive) continuation.resumeWithException(e)
                                }
                            }
                        }
                    })
                }
            }
        } catch (e: CancellationException) {
            throw e
        }
    }

    private fun parseErrorCode(body: String): String? {
        return try {
            val obj = JSONObject(body)
            if (obj.has("error")) {
                val err = obj.getJSONObject("error")
                if (err.has("code") && !err.isNull("code")) err.getString("code") else null
            } else {
                null
            }
        } catch (_: Exception) {
            null
        }
    }
}
