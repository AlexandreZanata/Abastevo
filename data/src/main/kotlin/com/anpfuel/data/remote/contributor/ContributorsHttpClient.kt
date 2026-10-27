package com.anpfuel.data.remote.contributor

import com.anpfuel.data.mapper.ContributorsMapper
import com.anpfuel.domain.contributor.ContributorException
import com.anpfuel.domain.contributor.ContributorPage
import com.anpfuel.domain.repository.ContributorsGateway
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

private const val BODY_BUDGET = 256L * 1024L + 1

/**
 * B-BR-CTB public contributors read (`GET /v1/contributors?cursor=`).
 *
 * Anonymous by contract (CTB-10): no session, proof or account id travels.
 * The list is edge-cacheable (`Cache-Control: public, max-age=300`), so no
 * `no-store` directive is sent. IO failures surface as [Offline] so the UI
 * can offer retry instead of a generic error.
 */
class ContributorsHttpClient(
    http: OkHttpClient,
    baseUrl: String,
) : ContributorsGateway {

    private val origin = baseUrl.toHttpUrl()
    private val client = http.newBuilder().followRedirects(false).followSslRedirects(false)
        .retryOnConnectionFailure(false).callTimeout(20, TimeUnit.SECONDS).build()

    override suspend fun page(cursor: String?): ContributorPage {
        if (cursor != null && cursor.length > 4096) throw invalid("cursor")
        val url = origin.newBuilder().addPathSegments("v1/contributors").apply {
            if (!cursor.isNullOrEmpty()) addQueryParameter("cursor", cursor)
        }.build()
        return try {
            withContext(Dispatchers.IO) {
                suspendCancellableCoroutine { continuation ->
                    val request = Request.Builder().url(url)
                        .header("Accept", "application/json").build()
                    val call = client.newCall(request)
                    continuation.invokeOnCancellation { call.cancel() }
                    call.enqueue(object : Callback {
                        override fun onFailure(call: Call, e: IOException) {
                            if (continuation.isActive) {
                                continuation.resumeWithException(ContributorException.Offline("contributors.offline"))
                            }
                        }

                        override fun onResponse(call: Call, response: Response) {
                            response.use {
                                if (!continuation.isActive) return
                                try {
                                    when (response.code) {
                                        200 -> Unit
                                        400, 401, 403, 404 -> throw ContributorException.Denied("contributors.denied")
                                        else -> throw ContributorException.Unavailable("contributors.unavailable")
                                    }
                                    val source = response.body?.source()
                                        ?: throw ContributorException.Unavailable("contributors.empty")
                                    if (source.request(BODY_BUDGET)) {
                                        throw ContributorException.Invalid("contributors.over-budget")
                                    }
                                    val raw = source.readUtf8()
                                    continuation.resume(
                                        try {
                                            ContributorsMapper.page(JSONObject(raw))
                                        } catch (e: ContributorException) {
                                            throw e
                                        } catch (e: Exception) {
                                            throw invalid("body")
                                        },
                                    )
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

    private fun invalid(part: String) =
        ContributorException.Invalid("contributors.invalid-$part")
}
