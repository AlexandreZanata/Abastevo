package com.anpfuel.data.remote

import com.anpfuel.data.mapper.WireFuelMapper
import com.anpfuel.domain.community.CityCommunitySummary
import com.anpfuel.domain.community.CommunityFeedPage
import com.anpfuel.domain.community.CommunitySummaryPrice
import com.anpfuel.domain.community.FeedQuery
import org.json.JSONArray
import org.json.JSONObject

/**
 * Wire-format encode for private community snapshots. The payload mirrors
 * the validated server shape so snapshots decode through the same strict
 * path (`CityCommunityFeedHttpClient.decode/decodeSummary`) and fail closed
 * on any drift. The advisory `refresh_after_seconds` hint is recorded as
 * observed (15); it never drives refresh policy and a future server value
 * fails the decode instead of being coerced.
 *
 * Pagination cursors are intentionally dropped: snapshots are first-page
 * only and deeper pages keep their live server cursors.
 */
object CommunityFeedCacheCodec {

    fun encodeFeed(query: FeedQuery, page: CommunityFeedPage): String {
        val items = JSONArray()
        page.items.forEach { item ->
            val row = JSONObject()
                .put("station_id", item.stationId)
                .put("station_name", item.stationName)
                .put("fuel_product", WireFuelMapper.toWire(item.fuel))
                .put("unit", item.unit)
                .put("currency", "BRL")
                .put("source", "COMMUNITY")
                .put("condition", "STANDARD")
                .put("amount_milli_brl", item.amountMilliBrl)
                .put("updated_at", item.updatedAt.toString())
                .put("expires_at", item.expiresAt.toString())
                .put("supporters", item.supporters)
                .put("confirmations", item.confirmations)
                .put("confidence", item.confidence)
                .put("version", item.version)
                .put("ratings_count", item.ratingsCount)
            if (item.ratingsAvg != null) row.put("ratings_avg", item.ratingsAvg) else row.put("ratings_avg", JSONObject.NULL)
            item.contribution?.let {
                row.put(
                    "contribution",
                    JSONObject()
                        .put("observation_id", it.observationId)
                        .put("decision_sequence", it.decisionSequence),
                )
            }
            items.put(row)
        }
        return JSONObject()
            .put("state", query.city.state.abbreviation)
            .put("municipality_code", query.city.code)
            .put("fuel_product", WireFuelMapper.toWire(query.fuel))
            .put(
                "sort",
                when (query.sort) {
                    com.anpfuel.domain.community.FeedSort.RECENT -> "recent"
                    com.anpfuel.domain.community.FeedSort.CHEAPEST -> "cheapest"
                    com.anpfuel.domain.community.FeedSort.BEST -> "best"
                    com.anpfuel.domain.community.FeedSort.WORST -> "worst"
                },
            )
            .put("items", items)
            .put("generated_at", page.generatedAt.toString())
            .put("refresh_after_seconds", 15)
            .toString()
    }

    fun encodeSummary(summary: CityCommunitySummary): String {
        val items = JSONArray()
        summary.items.forEach { price: CommunitySummaryPrice ->
            items.put(
                JSONObject()
                    .put("fuel_product", WireFuelMapper.toWire(price.fuel))
                    .put("unit", price.unit)
                    .put("currency", "BRL")
                    .put("source", "COMMUNITY")
                    .put("condition", "STANDARD")
                    .put("average_milli_brl", price.averageMilliBrl)
                    .put("minimum_milli_brl", price.minimumMilliBrl)
                    .put("maximum_milli_brl", price.maximumMilliBrl)
                    .put("station_count", price.stationCount)
                    .put("updated_at", price.updatedAt.toString())
                    .put("expires_at", price.expiresAt.toString()),
            )
        }
        return JSONObject()
            .put("state", summary.city.state.abbreviation)
            .put("municipality_code", summary.city.code)
            .put("generated_at", summary.generatedAt.toString())
            .put("items", items)
            .put("refresh_after_seconds", 15)
            .toString()
    }
}
