package com.anpfuel.data.local.forum

import com.anpfuel.data.mapper.ForumMapper
import com.anpfuel.domain.forum.ForumCategoriesResult
import com.anpfuel.domain.forum.ForumFeedPage
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.forum.ForumPostSummary
import org.json.JSONArray
import org.json.JSONObject

object ForumCacheCodec {

    fun encodeCategories(result: ForumCategoriesResult): String {
        val root = JSONObject()
        root.put("taxonomy_version", result.taxonomyVersion)
        val array = JSONArray()
        for (item in result.items) {
            val itemObj = JSONObject()
            itemObj.put("code", item.rawCode)
            itemObj.put("name", item.name)
            itemObj.put("description", item.description)
            array.put(itemObj)
        }
        root.put("items", array)
        return root.toString()
    }

    fun decodeCategories(json: String): ForumCategoriesResult {
        return ForumMapper.categories(JSONObject(json))
    }

    fun encodeFeed(page: ForumFeedPage): String {
        val root = JSONObject()
        root.put("snapshot", page.snapshot)
        root.put("fetched_at", page.fetchedAt)
        if (page.nextCursor != null) {
            root.put("next_cursor", page.nextCursor)
        } else {
            root.put("next_cursor", JSONObject.NULL)
        }
        val array = JSONArray()
        for (item in page.items) {
            array.put(encodeSummary(item))
        }
        root.put("items", array)
        return root.toString()
    }

    fun decodeFeed(json: String): ForumFeedPage {
        return ForumMapper.feed(JSONObject(json))
    }

    fun encodeDetail(detail: ForumPostDetail): String {
        val obj = encodeSummary(
            ForumPostSummary(
                id = detail.id,
                kind = detail.kind,
                rawKind = detail.rawKind,
                category = detail.category,
                rawCategory = detail.rawCategory,
                title = detail.title,
                excerpt = detail.excerpt,
                source = detail.source,
                publicAlias = detail.publicAlias,
                firstPublishedAt = detail.firstPublishedAt,
                updatedAt = detail.updatedAt,
                revision = detail.revision,
                commentsEnabled = detail.commentsEnabled,
                region = detail.region,
                fuelProduct = detail.fuelProduct,
                stationId = detail.stationId,
            ),
        )
        if (detail.bodyText != null) {
            obj.put("body_text", detail.bodyText)
        } else {
            obj.put("body_text", JSONObject.NULL)
        }
        if (detail.correction != null) {
            obj.put("correction", detail.correction)
        } else {
            obj.put("correction", JSONObject.NULL)
        }
        if (detail.sourcePublishedAt != null) {
            obj.put("source_published_at", detail.sourcePublishedAt)
        } else {
            obj.put("source_published_at", JSONObject.NULL)
        }
        return obj.toString()
    }

    fun decodeDetail(json: String): ForumPostDetail {
        return ForumMapper.detail(JSONObject(json))
    }

    private fun encodeSummary(summary: ForumPostSummary): JSONObject {
        val obj = JSONObject()
        obj.put("id", summary.id)
        obj.put("kind", summary.rawKind)
        obj.put("category", summary.rawCategory)
        obj.put("title", summary.title)
        obj.put("excerpt", summary.excerpt)
        obj.put("first_published_at", summary.firstPublishedAt)
        obj.put("updated_at", summary.updatedAt)
        obj.put("revision", summary.revision)
        obj.put("comments_enabled", summary.commentsEnabled)

        val source = summary.source
        if (source != null) {
            val sObj = JSONObject()
            sObj.put("name", source.name)
            sObj.put("url", source.url)
            sObj.put("attribution", source.attribution)
            if (source.licenseUrl != null) {
                sObj.put("license_url", source.licenseUrl)
            } else {
                sObj.put("license_url", JSONObject.NULL)
            }
            obj.put("source", sObj)
        } else {
            obj.put("source", JSONObject.NULL)
        }

        if (summary.publicAlias != null) {
            obj.put("public_alias", summary.publicAlias)
        } else {
            obj.put("public_alias", JSONObject.NULL)
        }

        val region = summary.region
        if (region != null) {
            val rObj = JSONObject()
            if (region.uf != null) rObj.put("uf", region.uf) else rObj.put("uf", JSONObject.NULL)
            if (region.municipalityIbge != null) rObj.put("municipality_ibge", region.municipalityIbge) else rObj.put("municipality_ibge", JSONObject.NULL)
            obj.put("region", rObj)
        } else {
            obj.put("region", JSONObject.NULL)
        }

        if (summary.fuelProduct != null) obj.put("fuel_product", summary.fuelProduct) else obj.put("fuel_product", JSONObject.NULL)
        if (summary.stationId != null) obj.put("station_id", summary.stationId) else obj.put("station_id", JSONObject.NULL)

        return obj
    }
}
