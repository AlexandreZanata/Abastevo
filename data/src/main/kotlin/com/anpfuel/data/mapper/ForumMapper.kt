package com.anpfuel.data.mapper

import com.anpfuel.domain.forum.ForumBulletinProvenance
import com.anpfuel.domain.forum.ForumBulletinRevision
import com.anpfuel.domain.forum.ForumCategoriesResult
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumCategoryItem
import com.anpfuel.domain.forum.ForumComment
import com.anpfuel.domain.forum.ForumCommentReceipt
import com.anpfuel.domain.forum.ForumCommentsPage
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumFeedPage
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.forum.ForumPostKind
import com.anpfuel.domain.forum.ForumPostSummary
import com.anpfuel.domain.forum.ForumRegion
import com.anpfuel.domain.forum.ForumRepliesPage
import com.anpfuel.domain.forum.ForumReportReceipt
import com.anpfuel.domain.forum.ForumSourceAttribution
import com.anpfuel.domain.forum.ForumTopicReceipt
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
        val supersedesPostId = if (json.has("supersedes_post_id") && !json.isNull("supersedes_post_id")) {
            json.getString("supersedes_post_id")
        } else {
            null
        }
        val bulletin = if (json.has("bulletin") && !json.isNull("bulletin")) {
            bulletinProvenance(json.getJSONObject("bulletin"))
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
            bulletin = bulletin,
            supersedesPostId = supersedesPostId,
        )
    }

    private fun bulletinProvenance(obj: JSONObject): ForumBulletinProvenance {
        return ForumBulletinProvenance(
            methodologyVersion = obj.optString("methodology_version", ""),
            templateVersion = obj.optString("template_version", ""),
            fuel = obj.optString("fuel", ""),
            unit = obj.optString("unit", ""),
            current = bulletinRevision(obj.getJSONObject("current")),
            previous = bulletinRevision(obj.getJSONObject("previous")),
        )
    }

    private fun bulletinRevision(obj: JSONObject): ForumBulletinRevision {
        return ForumBulletinRevision(
            revisionId = obj.optString("revision_id", ""),
            surveyStart = obj.optString("survey_start", ""),
            surveyEnd = obj.optString("survey_end", ""),
            publishedAt = obj.optString("published_at", ""),
            sourceUrl = obj.optString("source_url", ""),
            sourceChecksum = obj.optString("source_checksum", ""),
            parserVersion = obj.optString("parser_version", ""),
            sumMilliBrl = obj.optLong("sum_milli_brl", 0L),
            observationCount = obj.optLong("observation_count", 0L),
            sampledStations = obj.optLong("sampled_stations", 0L),
            sourceObservations = obj.optLong("source_observations", 0L),
            sourceStations = obj.optLong("source_stations", 0L),
            meanNumerator = obj.optLong("mean_numerator", 0L),
            meanDenominator = obj.optLong("mean_denominator", 0L),
            mean = obj.optString("mean", ""),
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

    fun comments(json: JSONObject): ForumCommentsPage {
        val itemsArray = json.optJSONArray("items") ?: JSONArray()
        val items = mutableListOf<ForumComment>()
        for (i in 0 until itemsArray.length()) {
            val obj = itemsArray.optJSONObject(i) ?: continue
            items.add(commentItem(obj))
        }
        val nextCursor = if (json.has("next_cursor") && !json.isNull("next_cursor")) {
            json.getString("next_cursor")
        } else {
            null
        }
        return ForumCommentsPage(
            items = items,
            nextCursor = nextCursor,
        )
    }

    fun replies(json: JSONObject): ForumRepliesPage {
        val itemsArray = json.optJSONArray("items") ?: JSONArray()
        val items = mutableListOf<ForumComment>()
        for (i in 0 until itemsArray.length()) {
            val obj = itemsArray.optJSONObject(i) ?: continue
            items.add(commentItem(obj))
        }
        val nextCursor = if (json.has("next_cursor") && !json.isNull("next_cursor")) {
            json.getString("next_cursor")
        } else {
            null
        }
        return ForumRepliesPage(
            items = items,
            nextCursor = nextCursor,
        )
    }

    fun commentItem(obj: JSONObject): ForumComment {
        val id = obj.optString("id", "")
        if (id.isEmpty()) throw ForumException.Invalid("forum.bad-id")
        val postId = obj.optString("post_id", "")
        val parentId = if (obj.has("parent_id") && !obj.isNull("parent_id")) obj.getString("parent_id") else null
        val publicAlias = if (obj.has("public_alias") && !obj.isNull("public_alias")) obj.getString("public_alias") else null
        val publishedAt = if (obj.has("published_at") && !obj.isNull("published_at")) obj.getString("published_at") else null

        return ForumComment(
            id = id,
            postId = postId,
            parentId = parentId,
            depth = obj.optInt("depth", 0),
            sequence = obj.optLong("sequence", 0L),
            publicAlias = publicAlias,
            bodyText = obj.optString("body_text", ""),
            createdAt = obj.optString("created_at", ""),
            updatedAt = obj.optString("updated_at", ""),
            publishedAt = publishedAt,
            replyCount = obj.optInt("reply_count", 0),
            revision = obj.optLong("revision", 1L),
        )
    }

    fun commentReceipt(json: JSONObject): ForumCommentReceipt {
        val obj = json.optJSONObject("comment") ?: json
        val id = obj.optString("id", "")
        if (id.isEmpty()) throw ForumException.Invalid("forum.bad-id")
        val postId = obj.optString("post_id", "")
        val parentId = if (obj.has("parent_id") && !obj.isNull("parent_id")) obj.getString("parent_id") else null
        val publishedAt = if (obj.has("published_at") && !obj.isNull("published_at")) obj.getString("published_at") else null

        return ForumCommentReceipt(
            id = id,
            postId = postId,
            parentId = parentId,
            depth = obj.optInt("depth", 0),
            sequence = obj.optLong("sequence", 0L),
            revision = obj.optLong("revision", 1L),
            publicationState = obj.optString("publication_state", "PENDING_REVIEW"),
            visibility = obj.optString("visibility", "VISIBLE"),
            bodyText = obj.optString("body_text", ""),
            createdAt = obj.optString("created_at", ""),
            updatedAt = obj.optString("updated_at", ""),
            publishedAt = publishedAt,
            replayed = obj.optBoolean("replayed", false),
        )
    }

    fun reportReceipt(json: JSONObject): ForumReportReceipt {
        val obj = json.optJSONObject("receipt") ?: json
        val reportId = obj.optString("report_id", "")
        if (reportId.isEmpty()) throw ForumException.Invalid("forum.bad-report-id")

        return ForumReportReceipt(
            reportId = reportId,
            caseId = obj.optString("case_id", ""),
            targetType = obj.optString("target_type", ""),
            targetId = obj.optString("target_id", ""),
            reason = obj.optString("reason", ""),
            status = obj.optString("status", ""),
            createdAt = obj.optString("created_at", ""),
            replayed = obj.optBoolean("replayed", false),
        )
    }

    fun topicReceipt(json: JSONObject): ForumTopicReceipt {
        val obj = json.optJSONObject("topic") ?: json
        val id = obj.optString("id", "")
        if (id.isEmpty()) throw ForumException.Invalid("forum.bad-id")
        val rawCategory = obj.optString("category", "")
        val category = ForumCategory.fromWire(rawCategory)
        val title = obj.optString("title", "")
        val bodyText = obj.optString("body_text", "")
        val revision = obj.optLong("revision", 1L)
        val publicationState = obj.optString("publication_state", "PENDING_REVIEW")
        val visibility = obj.optString("visibility", "VISIBLE")

        val region = if (obj.has("region") && !obj.isNull("region")) {
            val regObj = obj.getJSONObject("region")
            ForumRegion(
                uf = if (regObj.has("uf") && !regObj.isNull("uf")) regObj.getString("uf") else null,
                municipalityIbge = if (regObj.has("municipality_ibge") && !regObj.isNull("municipality_ibge")) regObj.getInt("municipality_ibge") else null,
            )
        } else {
            null
        }

        val fuelProduct = if (obj.has("fuel_product") && !obj.isNull("fuel_product")) obj.getString("fuel_product") else null
        val stationId = if (obj.has("station_id") && !obj.isNull("station_id")) obj.getString("station_id") else null
        val publicAlias = if (obj.has("public_alias") && !obj.isNull("public_alias")) obj.getString("public_alias") else null
        val createdAt = obj.optString("created_at", "")
        val updatedAt = obj.optString("updated_at", "")
        val publishedAt = if (obj.has("published_at") && !obj.isNull("published_at")) obj.getString("published_at") else null
        val replayed = obj.optBoolean("replayed", false)

        return ForumTopicReceipt(
            id = id,
            kind = obj.optString("kind", "TOPIC"),
            category = category,
            rawCategory = rawCategory,
            title = title,
            bodyText = bodyText,
            revision = revision,
            publicationState = publicationState,
            visibility = visibility,
            region = region,
            fuelProduct = fuelProduct,
            stationId = stationId,
            publicAlias = publicAlias,
            createdAt = createdAt,
            updatedAt = updatedAt,
            publishedAt = publishedAt,
            replayed = replayed,
        )
    }
}
