package com.anpfuel.data.remote.ranking

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

/** Typed transport failure: corrupt, oversize, foreign or refused bodies. */
class PriceRankingUnavailable(message: String? = null, cause: Throwable? = null) : IOException(message, cause)

private val canonicalId = Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")
private val ibgeCode = Regex("[0-9]{7}")
private val isoDay = Regex("[0-9]{4}-[0-9]{2}-[0-9]{2}")
private const val BODY_BUDGET = 1024L * 1024L + 1

data class PriceRankingParams(
    val fuel: String,
    val unit: String,
    val condition: String = "STANDARD",
    val geography: String,
    val uf: String?,
    val ibgeCode: String?,
    val metric: String,
    val groupBy: String,
    val sort: String,
    val revision: String?,
    val limit: Int,
    val cursor: String?,
)

data class PriceRankingStation(
    val stationId: String,
    val amountMilliBrl: Long,
    val collectedOn: String,
    val state: String?,
    val ibgeCode: String?,
)

data class PriceRankingArea(
    val level: String,
    val state: String,
    val ibgeCode: String,
    val meanNumerator: String,
    val meanDenominator: String,
    val minMilliBrl: Long,
    val maxMilliBrl: Long,
    val variationMilliBrl: Long,
    val observations: Long,
)

data class PriceRankingPage(
    val stations: List<PriceRankingStation>,
    val areas: List<PriceRankingArea>,
    val nextCursor: String?,
    val revisionId: String,
    val generatedAt: String,
)

/**
 * Bounded, anonymous typed price ranking reads over MEX-29.
 * Strict typed parsing with server order preserved (stable ties are
 * never re-sorted client-side); corrupt, oversize or foreign bodies
 * fail closed without disclosure. Cancellable, no redirects/retry, no
 * response cache. A repeated cursor fails closed instead of looping.
 */
class PriceRankingHttpClient(http: OkHttpClient, baseUrl: String) {
    private val origin = baseUrl.toHttpUrl()
    private val client = http.newBuilder().followRedirects(false).followSslRedirects(false)
        .retryOnConnectionFailure(false).callTimeout(20, TimeUnit.SECONDS).build()

    suspend fun rank(params: PriceRankingParams): PriceRankingPage {
        validate(params)
        if (params.cursor != null && params.cursor.isBlank()) throw PriceRankingUnavailable("bad cursor")
        val query = mutableMapOf(
            "source" to "ANP_WEEKLY",
            "fuel" to params.fuel, "unit" to params.unit, "condition" to params.condition,
            "geography" to params.geography, "metric" to params.metric,
            "group_by" to params.groupBy, "sort" to params.sort,
            "limit" to params.limit.toString(),
        )
        params.uf?.let { query["uf"] = it }
        params.ibgeCode?.let { query["ibge_code"] = it }
        params.revision?.let { query["revision_id"] = it }
        params.cursor?.let { query["cursor"] = it }
        val body = read(query)
        val page = decode(body)
        if (params.cursor != null && page.nextCursor == params.cursor) {
            throw PriceRankingUnavailable("cursor repeat")
        }
        return page
    }

    private fun validate(params: PriceRankingParams) {
        if (params.fuel.isBlank() || params.unit !in setOf("L", "M3", "KG_13")) throw PriceRankingUnavailable("bad product")
        if (params.condition != "STANDARD") throw PriceRankingUnavailable("unsupported condition")
        if (params.geography !in setOf("COUNTRY", "STATE", "MUNICIPALITY")) throw PriceRankingUnavailable("bad geography")
        if (params.geography == "STATE" && (params.uf == null || !Regex("^[A-Z]{2}$").matches(params.uf))) {
            throw PriceRankingUnavailable("bad uf")
        }
        if (params.geography == "MUNICIPALITY" && (params.ibgeCode == null || !ibgeCode.matches(params.ibgeCode))) {
            throw PriceRankingUnavailable("bad ibge")
        }
        if (params.metric !in setOf("OBSERVATION_PRICE", "AREA_MEAN")) throw PriceRankingUnavailable("bad metric")
        if (params.groupBy !in setOf("STATION", "MUNICIPALITY", "STATE")) throw PriceRankingUnavailable("bad group")
        if (params.sort !in setOf("ASC", "DESC")) throw PriceRankingUnavailable("bad sort")
        if (params.limit !in 1..100) throw PriceRankingUnavailable("bad limit")
        if (params.revision != null && !canonicalId.matches(params.revision)) throw PriceRankingUnavailable("bad revision")
    }

    private suspend fun read(params: Map<String, String?>): JSONObject {
        val url = origin.newBuilder().addPathSegments("v1/prices/ranking").apply {
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
                            if (continuation.isActive) continuation.resumeWithException(PriceRankingUnavailable("transport", e))
                        }

                        override fun onResponse(call: Call, response: Response) {
                            response.use {
                                if (!continuation.isActive) return
                                val parsed = try {
                                    if (!response.isSuccessful) throw PriceRankingUnavailable("status ${response.code}")
                                    val source = response.body?.source() ?: throw PriceRankingUnavailable("empty body")
                                    if (source.request(BODY_BUDGET)) throw PriceRankingUnavailable("body over budget")
                                    JSONObject(source.readUtf8())
                                } catch (e: Exception) {
                                    if (continuation.isActive) {
                                        continuation.resumeWithException(
                                            when (e) {
                                                is PriceRankingUnavailable -> e
                                                else -> PriceRankingUnavailable("body refused", e)
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

    private fun decode(body: JSONObject): PriceRankingPage {
        try {
            val lease = body.optJSONObject("source") ?: throw PriceRankingUnavailable("missing source")
            val revision = lease.optString("revision_id", "")
            if (!canonicalId.matches(revision)) throw PriceRankingUnavailable("bad revision")
            val generated = body.optString("generated_at", "")
            if (generated.isEmpty()) throw PriceRankingUnavailable("missing generated_at")
            val stations = body.optJSONArray("stations") ?: throw PriceRankingUnavailable("missing stations")
            if (stations.length() > 100) throw PriceRankingUnavailable("stations over bound")
            val stationRows = (0 until stations.length()).map { index ->
                val row = stations.getJSONObject(index)
                if (row.has("reporter") || row.has("price")) throw PriceRankingUnavailable("private field")
                val id = row.optString("station_id", "")
                val amount = if (row.isNull("amount_milli_brl")) -1 else row.optLong("amount_milli_brl", -1)
                val collected = row.optString("collected_on", "")
                if (!canonicalId.matches(id) || amount < 1 || !isoDay.matches(collected)) {
                    throw PriceRankingUnavailable("bad station row")
                }
                PriceRankingStation(
                    stationId = id, amountMilliBrl = amount, collectedOn = collected,
                    state = row.optString("state", null), ibgeCode = row.optString("ibge_code", null),
                )
            }
            if (stationRows.map { it.stationId }.distinct().size != stationRows.size) {
                throw PriceRankingUnavailable("duplicate station")
            }
            val areas = body.optJSONArray("areas") ?: throw PriceRankingUnavailable("missing areas")
            if (areas.length() > 100) throw PriceRankingUnavailable("areas over bound")
            val areaRows = (0 until areas.length()).map { index ->
                val row = areas.getJSONObject(index)
                val mean = row.optJSONObject("mean") ?: throw PriceRankingUnavailable("bad mean")
                PriceRankingArea(
                    level = row.optString("level", ""),
                    state = row.optString("state", ""),
                    ibgeCode = row.optString("ibge_code", ""),
                    meanNumerator = mean.optString("numerator", ""),
                    meanDenominator = mean.optString("denominator", ""),
                    minMilliBrl = row.optLong("min_milli_brl", -1),
                    maxMilliBrl = row.optLong("max_milli_brl", -1),
                    variationMilliBrl = row.optLong("variation_milli_brl", -1),
                    observations = row.optLong("observations", 0),
                ).also {
                    if (it.level !in setOf("MUNICIPALITY", "STATE") || it.meanNumerator.isEmpty() ||
                        it.meanDenominator.isEmpty() || it.minMilliBrl < 1 || it.observations < 1
                    ) {
                        throw PriceRankingUnavailable("bad area row")
                    }
                }
            }
            val next = if (body.isNull("next_cursor")) null else body.optString("next_cursor", null)?.takeIf { it.isNotEmpty() }
            return PriceRankingPage(stationRows, areaRows, next, revision, generated)
        } catch (e: PriceRankingUnavailable) {
            throw e
        } catch (e: Exception) {
            throw PriceRankingUnavailable("ranking refused", e)
        }
    }
}
