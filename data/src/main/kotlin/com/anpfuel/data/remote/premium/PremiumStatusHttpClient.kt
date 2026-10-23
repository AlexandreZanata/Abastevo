package com.anpfuel.data.remote.premium

import com.anpfuel.domain.premium.PremiumState
import com.anpfuel.domain.premium.PremiumStatus
import com.anpfuel.domain.premium.PremiumStatusException
import com.anpfuel.domain.repository.PremiumStatusGateway
import java.io.IOException
import java.time.Instant
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
import org.json.JSONArray
import org.json.JSONObject

private val JSON = "application/json; charset=utf-8".toMediaType()
private const val BODY_BUDGET = 64L * 1024L + 1
private const val MAX_CAPABILITIES = 32
private const val MAX_CAPABILITY_CHARS = 64
private val allowedCapabilities = setOf(
    "cloud.sync",
    "cloud.backup",
    "cloud.restore",
    "alerts.advanced",
    "professional.write",
    "professional.report",
    "vehicles.multiple",
)

/**
 * MEX-16 private Premium status read (`POST /v1/premium/status`).
 *
 * Only the current family/access proof travels; no account id, Premium
 * flag or purchase token is accepted from callers. Responses are
 * private and never cached. The client validates the frozen status
 * shape and keeps unknown states/capabilities explicit rather than
 * granting access or crashing on forward-compatible values.
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

    private fun decode(root: JSONObject): PremiumStatus {
        try {
            if (root.optInt("policy_version", -1) != 1) throw invalid("policy")
            val state = PremiumState.parse(root.getString("state"))
            val reason = root.getString("reason")
            if (reason.length > 64) throw invalid("reason")
            val capabilitiesRaw = root.optJSONArray("capabilities") ?: JSONArray()
            if (capabilitiesRaw.length() > MAX_CAPABILITIES) throw invalid("capabilities")
            val capabilities = mutableSetOf<String>()
            for (index in 0 until capabilitiesRaw.length()) {
                val value = capabilitiesRaw.opt(index)
                if (value !is String || value.isBlank() || value.length > MAX_CAPABILITY_CHARS ||
                    value !in allowedCapabilities
                ) {
                    throw invalid("capability")
                }
                capabilities.add(value)
            }
            val validUntil = if (root.isNull("valid_until")) {
                null
            } else {
                val raw = root.getString("valid_until")
                Instant.parse(raw)
                raw
            }
            if (root.opt("retained_data_access") != true) throw invalid("retained")
            if (root.opt("payments_configured") !is Boolean) throw invalid("payments")
            val management = if (root.has("management_url")) {
                if (root.getString("management_url") != PremiumStatus.MANAGEMENT_URL) throw invalid("management")
                PremiumStatus.MANAGEMENT_URL
            } else {
                PremiumStatus.MANAGEMENT_URL
            }
            return PremiumStatus(
                policyVersion = 1,
                state = state,
                reason = reason,
                capabilities = capabilities.toSet(),
                validUntil = validUntil,
                retainedDataAccess = true,
                paymentsConfigured = root.getBoolean("payments_configured"),
                managementUrl = management,
            )
        } catch (e: PremiumStatusException) {
            throw e
        } catch (e: Exception) {
            throw invalid("body", e)
        }
    }

    private fun invalid(part: String, cause: Throwable? = null) =
        PremiumStatusException.Invalid("premium.invalid-$part", cause)

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
