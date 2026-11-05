package com.anpfuel.data.remote.forum

import com.anpfuel.data.mapper.ForumMapper
import com.anpfuel.domain.forum.ForumCategoriesResult
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumFeedPage
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.forum.ForumPostKind
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
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import org.json.JSONObject

private const val BODY_BUDGET = 512L * 1024L + 1 // 512 KiB byte cap

/**
 * Public HTTP transport client for Forum Abastevo (FOR-07 / B-BR-FOR-01..07).
 *
 * Anonymous reads: no session tokens, auth proofs or contributor IDs travel.
 * Bounded by strict byte budget (512 KiB) and 15s timeout.
 */
class ForumHttpClient(
    http: OkHttpClient,
    baseUrl: String,
) : ForumGateway {

    private val origin = baseUrl.trimEnd('/').toHttpUrl()
    private val client = http.newBuilder()
        .followRedirects(false)
        .followSslRedirects(false)
        .retryOnConnectionFailure(false)
        .callTimeout(15, TimeUnit.SECONDS)
        .build()

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
            ForumMapper.detail(JSONObject(raw))
        } catch (e: ForumException) {
            throw e
        } catch (e: Exception) {
            throw ForumException.Invalid("forum.bad-response")
        }
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
                                        200 -> continuation.resume(body)
                                        404 -> {
                                            val errCode = parseErrorCode(body)
                                            if (errCode == "forum.disabled") {
                                                throw ForumException.Disabled("forum.disabled")
                                            } else {
                                                throw ForumException.NotFound("forum.not-found")
                                            }
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
