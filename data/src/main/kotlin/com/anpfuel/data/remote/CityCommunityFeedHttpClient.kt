package com.anpfuel.data.remote

import com.anpfuel.data.mapper.WireFuelMapper
import com.anpfuel.domain.community.CityCommunityFeedReader
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
class CityCommunityFeedHttpClient(private val client: OkHttpClient, private val origin: String) : CityCommunityFeedReader {
    override suspend fun read(query: FeedQuery, cursor: String?): CommunityFeedPage {
        val url = (origin.trimEnd('/') + "/v1/community/feed").toHttpUrl().newBuilder()
            .addQueryParameter("state", query.city.state.abbreviation)
            .addQueryParameter("municipality_code", query.city.code)
            .addQueryParameter("fuel_product", WireFuelMapper.toWire(query.fuel))
            .addQueryParameter("sort", if (query.sort == FeedSort.RECENT) "recent" else "cheapest")
            .addQueryParameter("limit", "20")
            .apply { cursor?.let { addQueryParameter("cursor", it) } }.build()
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
        return decode(payload, query)
    }

    companion object {
        internal fun decode(payload: String, query: FeedQuery): CommunityFeedPage {
            require(payload.length <= 256 * 1024)
            // Limit nesting before org.json recursively parses an untrusted body.
            var depth = 0
            var quoted = false
            var escaped = false
            for (character in payload) {
                if (quoted) {
                    when {
                        escaped -> escaped = false
                        character == '\\' -> escaped = true
                        character == '"' -> quoted = false
                    }
                } else {
                    when (character) {
                        '"' -> quoted = true
                        '{', '[' -> { depth++; require(depth <= 12) }
                        '}', ']' -> { depth--; require(depth >= 0) }
                    }
                }
            }
            require(depth == 0 && !quoted)
            val page = JSONObject(payload)
            require(page.getString("state") == query.city.state.abbreviation && page.getString("municipality_code") == query.city.code)
            require(page.getString("fuel_product") == WireFuelMapper.toWire(query.fuel))
            require(page.getString("sort") == if (query.sort == FeedSort.RECENT) "recent" else "cheapest")
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
