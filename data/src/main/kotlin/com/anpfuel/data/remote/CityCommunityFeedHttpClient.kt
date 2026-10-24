package com.anpfuel.data.remote

import com.anpfuel.data.mapper.WireFuelMapper
import com.anpfuel.domain.community.*
import com.anpfuel.domain.community.CommunityFeedItem
import com.anpfuel.domain.community.CommunityFeedPage
import com.anpfuel.domain.community.FeedQuery
import com.anpfuel.domain.community.FeedSort
import java.io.IOException
import java.time.Instant
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.suspendCancellableCoroutine
import okhttp3.Call
import okhttp3.Callback
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import org.json.JSONObject

/** Anonymous public feed. Cancellation terminates the HTTP call on tab/city change. */
class CityCommunityFeedHttpClient(private val client: OkHttpClient, private val origin: String) : CityCommunityFeedReader, CityCommunitySummaryReader {
    override suspend fun read(query: FeedQuery, cursor: String?): CommunityFeedPage {
        val url = (origin.trimEnd('/') + "/v1/community/feed").toHttpUrl().newBuilder()
            .addQueryParameter("state", query.city.state.abbreviation)
            .addQueryParameter("municipality_code", query.city.code)
            .addQueryParameter("fuel_product", WireFuelMapper.toWire(query.fuel))
            .addQueryParameter("sort", when (query.sort) {
                FeedSort.RECENT -> "recent"
                FeedSort.CHEAPEST -> "cheapest"
                FeedSort.BEST -> "best"
                FeedSort.WORST -> "worst"
            })
            .addQueryParameter("limit", "20")
            .apply { cursor?.let { addQueryParameter("cursor", it) } }.build()
        return decode(fetch(url), query)
    }

    override suspend fun readSummary(city: FeedCity): CityCommunitySummary {
        val url = (origin.trimEnd('/') + "/v1/community/summary").toHttpUrl().newBuilder()
            .addQueryParameter("state", city.state.abbreviation).addQueryParameter("municipality_code", city.code).build()
        return decodeSummary(fetch(url), city)
    }

    private suspend fun fetch(url: okhttp3.HttpUrl): String {
        val call = client.newCall(Request.Builder().url(url).header("Accept", "application/json").build())
        val payload = suspendCancellableCoroutine<String> { continuation ->
            continuation.invokeOnCancellation { call.cancel() }
            call.enqueue(object : Callback {
                override fun onFailure(call: Call, e: IOException) {
                    if (continuation.isActive) continuation.resumeWithException(IOException("community feed unavailable"))
                }
                override fun onResponse(call: Call, response: Response) {
                    response.use {
                        try {
                            if (!response.isSuccessful) throw IOException("community feed HTTP ${response.code}")
                            val body = response.body ?: throw IOException("community feed empty body")
                            // Bound memory even when the server/proxy sends an invalid response.
                            val source = body.source()
                            source.request(256 * 1024 + 1L)
                            if (source.buffer.size > 256 * 1024) throw IOException("community feed oversized")
                            val text = source.readUtf8()
                            if (continuation.isActive) continuation.resume(text)
                        } catch (error: Exception) {
                            if (continuation.isActive) continuation.resumeWithException(IOException("community feed unavailable"))
                        }
                    }
                }
            })
        }
        return payload
    }

    companion object {
        internal fun decodeSummary(payload: String, city: FeedCity): CityCommunitySummary {
            checkEnvelope(payload)
            val page = JSONObject(payload)
            require(page.getString("state") == city.state.abbreviation && page.getString("municipality_code") == city.code)
            requireRefreshHint(page)
            val generated = Instant.parse(page.getString("generated_at"))
            val rows = page.getJSONArray("items"); require(rows.length() <= 8)
            fun integer(row: JSONObject, field: String): Long = row.get(field).let {
                require(it is Number && it.toString().matches(Regex("[0-9]+")))
                it.toString().toLong()
            }
            val items = (0 until rows.length()).map { index ->
                val row = rows.getJSONObject(index)
                require(row.getString("source") == "COMMUNITY" && row.getString("currency") == "BRL" && row.getString("condition") == "STANDARD")
                val count = integer(row,"station_count"); require(count in 1..Int.MAX_VALUE.toLong())
                CommunitySummaryPrice(WireFuelMapper.fromWire(row.getString("fuel_product")).getOrThrow(),
                    integer(row,"average_milli_brl"), integer(row,"minimum_milli_brl"), integer(row,"maximum_milli_brl"), count.toInt(),
                    Instant.parse(row.getString("updated_at")), Instant.parse(row.getString("expires_at"))).also {
                    require(it.unit == row.getString("unit"))
                }
            }
            return CityCommunitySummary(city, items, generated)
        }
        internal fun requireRefreshHint(page: JSONObject) {
            // Advisory only: bounded server hints are accepted, never used
            // as a fixed UI refresh promise or as a cache policy.
            require(page.getInt("refresh_after_seconds") in 0..3600)
        }
        private fun checkEnvelope(payload: String) {
            require(payload.length <= 256 * 1024)
            var depth = 0; var quoted = false; var escaped = false
            for(character in payload) {
                if(quoted) {when { escaped -> escaped=false; character=='\\' -> escaped=true; character=='"' -> quoted=false }}
                else when(character) { '"' -> quoted=true; '{','[' -> {depth++;require(depth<=12)}; '}',']' -> {depth--;require(depth>=0)} }
            }
            require(depth == 0 && !quoted)
        }
        internal fun decode(payload: String, query: FeedQuery): CommunityFeedPage {
            checkEnvelope(payload)
            val page = JSONObject(payload)
            require(page.getString("state") == query.city.state.abbreviation && page.getString("municipality_code") == query.city.code)
            require(page.getString("fuel_product") == WireFuelMapper.toWire(query.fuel))
            require(page.getString("sort") == when (query.sort) {
                FeedSort.RECENT -> "recent"
                FeedSort.CHEAPEST -> "cheapest"
                FeedSort.BEST -> "best"
                FeedSort.WORST -> "worst"
            })
            val rows = page.getJSONArray("items")
            require(rows.length() <= 20)
            val generated = Instant.parse(page.getString("generated_at"))
            val items = (0 until rows.length()).map { index ->
                val row = rows.getJSONObject(index)
                require(row.getString("source") == "COMMUNITY" && row.getString("currency") == "BRL" && row.getString("condition") == "STANDARD")
                val item = CommunityFeedItem(
                    stationId = row.getString("station_id"), stationName = row.getString("station_name"),
                    fuel = WireFuelMapper.fromWire(row.getString("fuel_product")).getOrThrow(),
                    amountMilliBrl = row.get("amount_milli_brl").also { require(it is Number) }.toString().also { require(it.matches(Regex("[0-9]+"))) }.toLong(),
                    updatedAt = Instant.parse(row.getString("updated_at")), expiresAt = Instant.parse(row.getString("expires_at")),
                    supporters = row.getInt("supporters"), confirmations = row.getInt("confirmations"),
                    confidence = row.getString("confidence"), version = row.getLong("version"),
                    contribution = row.optJSONObject("contribution")?.let { ref ->
                        com.anpfuel.domain.community.ContributionReference(ref.getString("observation_id"),ref.getLong("decision_sequence"))
                    },
                    ratingsCount = row.optInt("ratings_count"),
                    ratingsAvg = if (row.isNull("ratings_avg")) null else row.getDouble("ratings_avg"),
                )
                require(item.fuel == query.fuel && item.unit == row.getString("unit") && item.updatedAt <= generated)
                item
            }
            require(items.distinctBy { it.stationId }.size == items.size)
            val cursor = if (page.isNull("next_cursor")) null else page.getString("next_cursor").also { require(it.isNotBlank() && it.length <= 4096) }
            return CommunityFeedPage(items, cursor, generated)
        }
    }
}
