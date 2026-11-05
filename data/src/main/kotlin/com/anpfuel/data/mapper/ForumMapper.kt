package com.anpfuel.data.mapper

import com.anpfuel.domain.forum.ForumCategoriesResult
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumCategoryItem
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumFeedPage
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.forum.ForumPostKind
import com.anpfuel.domain.forum.ForumPostSummary
import com.anpfuel.domain.forum.ForumRegion
import com.anpfuel.domain.forum.ForumSourceAttribution
import org.json.JSONArray
import org.json.JSONObject

object ForumMapper {

    fun categories(json: JSONObject): ForumCategoriesResult {
        val itemsArray = json.optJSONArray("items") ?: JSONArray()
        val items = mutableListOf<ForumCategoryItem>()
        for (i in 0 until itemsArray.length()) {
            val obj = itemsArray.optJSONObject(i) ?: continue
            val rawCode = obj.optString("code", "")
            items.add(
                ForumCategoryItem(
                    code = ForumCategory.fromWire(rawCode),
                    rawCode = rawCode,
                    name = obj.optString("name", ""),
                    description = obj.optString("description", ""),
                ),
            )
        }
        return ForumCategoriesResult(
            items = items,
            taxonomyVersion = json.optInt("taxonomy_version", 1),
        )
    }

    fun feed(json: JSONObject): ForumFeedPage {
        val itemsArray = json.optJSONArray("items") ?: JSONArray()
        val items = mutableListOf<ForumPostSummary>()
        for (i in 0 until itemsArray.length()) {
            val obj = itemsArray.optJSONObject(i) ?: continue
            items.add(postSummary(obj))
        }
        val nextCursor = if (json.has("next_cursor") && !json.isNull("next_cursor")) {
            json.getString("next_cursor")
        } else {
            null
        }
        return ForumFeedPage(
            items = items,
            nextCursor = nextCursor,
            snapshot = json.optString("snapshot", ""),
            fetchedAt = json.optString("fetched_at", ""),
        )
    }

    fun detail(json: JSONObject): ForumPostDetail {
        val summary = postSummary(json)
        val bodyText = if (json.has("body_text") && !json.isNull("body_text")) {
            json.getString("body_text")
        } else {
            null
        }
        val correction = if (json.has("correction") && !json.isNull("correction")) {
            json.getString("correction")
        } else {
            null
        }
        val sourcePubAt = if (json.has("source_published_at") && !json.isNull("source_published_at")) {
            json.getString("source_published_at")
        } else {
            null
        }
        return ForumPostDetail(
            id = summary.id,
            kind = summary.kind,
            rawKind = summary.rawKind,
            category = summary.category,
            rawCategory = summary.rawCategory,
            title = summary.title,
            excerpt = summary.excerpt,
            bodyText = bodyText,
            source = summary.source,
            publicAlias = summary.publicAlias,
            sourcePublishedAt = sourcePubAt,
            firstPublishedAt = summary.firstPublishedAt,
            updatedAt = summary.updatedAt,
            revision = summary.revision,
            commentsEnabled = summary.commentsEnabled,
            correction = correction,
            region = summary.region,
            fuelProduct = summary.fuelProduct,
            stationId = summary.stationId,
        )
    }

    fun postSummary(obj: JSONObject): ForumPostSummary {
        val rawKind = obj.optString("kind", "EDITORIAL")
        val rawCategory = obj.optString("category", "FUELS")
        val id = obj.optString("id", "")
        if (id.isEmpty()) throw ForumException.Invalid("forum.bad-id")

        val source = if (obj.has("source") && !obj.isNull("source")) {
            val sObj = obj.getJSONObject("source")
            ForumSourceAttribution(
                name = sObj.optString("name", ""),
                url = sObj.optString("url", ""),
                attribution = sObj.optString("attribution", ""),
                licenseUrl = if (sObj.has("license_url") && !sObj.isNull("license_url")) sObj.getString("license_url") else null,
            )
        } else {
            null
        }

        val region = if (obj.has("region") && !obj.isNull("region")) {
            val rObj = obj.getJSONObject("region")
            ForumRegion(
                uf = if (rObj.has("uf") && !rObj.isNull("uf")) rObj.getString("uf") else null,
                municipalityIbge = if (rObj.has("municipality_ibge") && !rObj.isNull("municipality_ibge")) rObj.getInt("municipality_ibge") else null,
            )
        } else {
            null
        }

        val publicAlias = if (obj.has("public_alias") && !obj.isNull("public_alias")) obj.getString("public_alias") else null
        val fuelProduct = if (obj.has("fuel_product") && !obj.isNull("fuel_product")) obj.getString("fuel_product") else null
        val stationId = if (obj.has("station_id") && !obj.isNull("station_id")) obj.getString("station_id") else null

        return ForumPostSummary(
            id = id,
            kind = ForumPostKind.fromWire(rawKind),
            rawKind = rawKind,
            category = ForumCategory.fromWire(rawCategory),
            rawCategory = rawCategory,
            title = obj.optString("title", ""),
            excerpt = obj.optString("excerpt", ""),
            source = source,
            publicAlias = publicAlias,
            firstPublishedAt = obj.optString("first_published_at", ""),
            updatedAt = obj.optString("updated_at", ""),
            revision = obj.optLong("revision", 1L),
            commentsEnabled = obj.optBoolean("comments_enabled", true),
            region = region,
            fuelProduct = fuelProduct,
            stationId = stationId,
        )
    }
}
