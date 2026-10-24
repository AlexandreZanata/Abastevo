package com.anpfuel.data.remote.charging

import com.anpfuel.domain.charging.ChargingCoverage
import com.anpfuel.domain.charging.ChargingPlace
import com.anpfuel.domain.charging.ChargingPlacePage
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

private val canonicalId = Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")
private const val BODY_BUDGET = 1024L * 1024L + 1

/** Bounded, anonymous public charging snapshot reads over EE-09.
 * Strict typed parsing with unknown-enum retention and no-store honored:
 * no response cache exists, every fetch hits the wire, and corrupt,
 * oversize or foreign bodies fail closed without disclosure. Only
 * validated EE-09 bodies reach the Room snapshot importer; licensed
 * operator endpoints are never called here. */
class ChargingSnapshotHttpClient(http: OkHttpClient, baseUrl: String) {
    private val origin = baseUrl.toHttpUrl()
    private val client = http.newBuilder().followRedirects(false).followSslRedirects(false)
        .retryOnConnectionFailure(false).callTimeout(20, TimeUnit.SECONDS).build()

    suspend fun places(profile: String, revision: String? = null, limit: Int, cursor: String?): ChargingPlacePage {
        if (profile.isBlank() || limit !in 1..100 || (cursor != null && cursor.isBlank())) {
            throw ChargingSnapshotUnavailable("bad page filter")
        }
        val params = mutableMapOf("profile" to profile, "limit" to limit.toString(), "cursor" to cursor, "revision" to revision)
        val body = read("places", params)
        val page = ChargingSnapshotJsonCodec.decodePlacePage(body)
        if (cursor != null && page.nextCursor == cursor) throw ChargingSnapshotUnavailable("cursor repeat")
        return page
    }

    suspend fun placeDetail(id: String): ChargingPlace {
        if (!canonicalId.matches(id)) throw ChargingSnapshotUnavailable("bad place id")
        return ChargingSnapshotJsonCodec.decodePlaceDetail(read("places/$id", emptyMap()))
    }

    suspend fun coverage(profile: String): ChargingCoverage {
        if (profile.isBlank()) throw ChargingSnapshotUnavailable("bad coverage filter")
        return ChargingSnapshotJsonCodec.decodeCoverage(read("coverage", mapOf("profile" to profile)))
    }

    private suspend fun read(route: String, params: Map<String, String?>): JSONObject {
        val url = origin.newBuilder().addPathSegments("v1/charging/$route").apply {
            params.forEach { (key, value) -> value?.let { addQueryParameter(key, it) } }
        }.build()
        return try {
            withContext(Dispatchers.IO) {
                suspendCancellableCoroutine { continuation ->
                    val request = Request.Builder().url(url)
                        .header("Accept", "application/json")
                        .header("Cache-Control", "no-store")
                        .get().build()
                    val call = client.newCall(request)
                    continuation.invokeOnCancellation { call.cancel() }
                    call.enqueue(object : Callback {
                        override fun onFailure(call: Call, e: IOException) {
                            if (continuation.isActive) continuation.resumeWithException(ChargingSnapshotUnavailable("transport", e))
                        }

                        override fun onResponse(call: Call, response: Response) {
                            response.use {
                                if (!continuation.isActive) return
                                val parsed = try {
                                    if (!response.isSuccessful) throw ChargingSnapshotUnavailable("status ${response.code}")
                                    val source = response.body?.source() ?: throw ChargingSnapshotUnavailable("empty body")
                                    if (source.request(BODY_BUDGET)) throw ChargingSnapshotUnavailable("body over budget")
                                    JSONObject(source.readUtf8())
                                } catch (e: Exception) {
                                    if (continuation.isActive) {
                                        continuation.resumeWithException(
                                            when (e) {
                                                is ChargingSnapshotUnavailable -> e
                                                else -> ChargingSnapshotUnavailable("body refused", e)
                                            },
                                        )
                                    }
                                    return
                                }
                                continuation.resume(parsed)
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
