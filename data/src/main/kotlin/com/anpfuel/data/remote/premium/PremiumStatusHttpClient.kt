package com.anpfuel.data.remote.premium

import com.anpfuel.domain.premium.PremiumStatus
import com.anpfuel.domain.premium.PremiumStatusException
import com.anpfuel.domain.repository.PremiumStatusGateway
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

private val JSON = "application/json; charset=utf-8".toMediaType()
private const val BODY_BUDGET = 64L * 1024L + 1

/**
 * MEX-16 private Premium status read (`POST /v1/premium/status`).
 *
 * Only the current family/access proof travels; no account id, Premium
 * flag or purchase token is accepted from callers. Responses are
 * private and never cached. The client validates the frozen status
 * shape and keeps unknown states/capabilities explicit rather than
 * granting access or crashing on forward-compatible values. Store offers
 * and active tiers are decoded by [PremiumStatusJson].
 */
class PremiumStatusHttpClient(
    http: OkHttpClient,
    baseUrl: String,
) : PremiumStatusGateway {

    private val origin = baseUrl.toHttpUrl()
    private val client = http.newBuilder().followRedirects(false).followSslRedirects(false)
        .retryOnConnectionFailure(false).callTimeout(20, TimeUnit.SECONDS).build()

    override suspend fun status(familyId: String, accessToken: String): PremiumStatus {
        if (familyId.isBlank() || accessToken.isBlank()) throw PremiumStatusException.SignedOut()
        val body = JSONObject().put("family_id", familyId).put("access_token", accessToken).toString()
        val root = post(body)
        return decode(root)
    }

    private fun decode(root: JSONObject): PremiumStatus = PremiumStatusJson.decode(root)

    private fun invalid(part: String, cause: Throwable? = null) = PremiumStatusJson.invalid(part, cause)

    private suspend fun post(body: String): JSONObject {
        val url = origin.newBuilder().addPathSegments("v1/premium/status").build()
        return try {
            withContext(Dispatchers.IO) {
                suspendCancellableCoroutine { continuation ->
                    val request = Request.Builder().url(url)
                        .header("Accept", "application/json")
                        .header("Cache-Control", "no-store")
                        .post(body.toByteArray(Charsets.UTF_8).toRequestBody(JSON)).build()
                    val call = client.newCall(request)
                    continuation.invokeOnCancellation { call.cancel() }
                    call.enqueue(object : Callback {
                        override fun onFailure(call: Call, e: IOException) {
                            if (continuation.isActive) continuation.resumeWithException(e)
                        }

                        override fun onResponse(call: Call, response: Response) {
                            response.use {
                                if (!continuation.isActive) return
                                try {
                                    when (response.code) {
                                        200 -> Unit
                                        400, 401, 403, 404 -> throw PremiumStatusException.Denied("premium.denied")
                                        else -> throw PremiumStatusException.Unavailable("premium.unavailable")
                                    }
                                    val source = response.body?.source()
                                        ?: throw PremiumStatusException.Unavailable("premium.empty")
                                    if (source.request(BODY_BUDGET)) {
                                        throw PremiumStatusException.Invalid("premium.over-budget")
                                    }
                                    val raw = source.readUtf8()
                                    continuation.resume(
                                        try {
                                            JSONObject(raw)
                                        } catch (e: Exception) {
                                            throw invalid("body", e)
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
}
