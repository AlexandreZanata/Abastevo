package com.anpfuel.data.local.notifications

import com.anpfuel.data.mapper.WireFuelMapper
import com.anpfuel.domain.notification.AppNotification
import com.anpfuel.domain.notification.AppNotificationContent
import com.anpfuel.domain.notification.AppNotificationSource
import com.anpfuel.domain.notification.FuelDrop
import com.anpfuel.domain.valueobject.FuelProduct
import java.time.Instant
import org.json.JSONArray
import org.json.JSONObject

/**
 * Device JSON for notifications (local inbox, account page cache) and the
 * decoder of B-BR-NOTIF-03 server items. Unknown or malformed items decode to
 * null and are skipped, so new server kinds never break the screen.
 */
internal object NotificationJsonCodec {

    private const val MAX_TEXT = 160

    fun encode(items: List<AppNotification>): String = JSONArray().apply { items.forEach { put(encode(it)) } }.toString()

    fun decodeList(raw: String?): List<AppNotification> {
        if (raw.isNullOrBlank()) return emptyList()
        return try {
            val array = JSONArray(raw)
            (0 until array.length()).mapNotNull { decode(array.optJSONObject(it)) }
        } catch (_: Exception) {
            emptyList()
        }
    }

    fun encode(item: AppNotification): JSONObject = JSONObject()
        .put("id", item.id)
        .put("source", item.source.name)
        .put("at", item.createdAtMillis)
        .put("read", item.read)
        .put("content", encodeContent(item.content))

    fun decode(json: JSONObject?): AppNotification? {
        json ?: return null
        return try {
            val content = decodeContent(json.getJSONObject("content")) ?: return null
            AppNotification(
                id = json.getString("id").also { require(it.isNotBlank() && it.length <= 200) },
                source = AppNotificationSource.valueOf(json.getString("source")),
                content = content,
                createdAtMillis = json.getLong("at"),
                read = json.optBoolean("read", false),
            )
        } catch (_: Exception) {
            null
        }
    }

    private fun encodeContent(content: AppNotificationContent): JSONObject = when (content) {
        is AppNotificationContent.ForumReply -> JSONObject().put("k", "forum_reply")
            .put("topic_id", content.topicId).put("topic_title", content.topicTitle).put("author_name", content.authorName)
        is AppNotificationContent.ContributionDecided -> JSONObject().put("k", "contribution")
            .put("station_name", content.stationName).put("fuel", content.fuel?.name ?: JSONObject.NULL)
            .put("validated", content.validated)
        is AppNotificationContent.CityAverageDrop -> JSONObject().put("k", "city_drop")
            .put("municipality", content.municipality).put("state", content.stateAbbreviation)
            .put("drops", JSONArray().apply {
                content.drops.forEach { put(JSONObject().put("fuel", it.fuel.name).put("current", it.currentMilli).put("drop", it.dropMilli)) }
            })
        is AppNotificationContent.VehiclePriceDrop -> JSONObject().put("k", "vehicle_drop")
            .put("vehicle", content.vehicleName).put("price", content.priceFormatted)
        is AppNotificationContent.NewSurveyWeek -> JSONObject().put("k", "new_week")
            .put("start", content.startIso).put("end", content.endIso)
    }

    private fun decodeContent(json: JSONObject): AppNotificationContent? = when (json.optString("k")) {
        "forum_reply" -> AppNotificationContent.ForumReply(
            json.getString("topic_id"), json.getString("topic_title").take(MAX_TEXT), json.getString("author_name").take(MAX_TEXT),
        )
        "contribution" -> AppNotificationContent.ContributionDecided(
            json.getString("station_name").take(MAX_TEXT),
            json.optString("fuel").takeIf { it.isNotBlank() && it != "null" }?.let { runCatching { FuelProduct.valueOf(it) }.getOrNull() },
            json.getBoolean("validated"),
        )
        "city_drop" -> {
            val drops = json.getJSONArray("drops")
            AppNotificationContent.CityAverageDrop(
                json.getString("municipality"), json.getString("state"),
                (0 until drops.length()).map { index ->
                    val drop = drops.getJSONObject(index)
                    FuelDrop(FuelProduct.valueOf(drop.getString("fuel")), drop.getLong("current"), drop.getLong("drop"))
                },
            )
        }
        "vehicle_drop" -> AppNotificationContent.VehiclePriceDrop(json.getString("vehicle"), json.getString("price"))
        "new_week" -> AppNotificationContent.NewSurveyWeek(json.getString("start"), json.getString("end"))
        else -> null
    }

    /** One B-BR-NOTIF-03 list item; null for an unknown kind or a malformed payload. */
    fun decodeServerItem(json: JSONObject): AppNotification? = decodeServerItemOrThrow(json)

    private fun decodeServerItemOrThrow(json: JSONObject): AppNotification? = try {
        val id = json.getString("notification_id").also { require(UUID.matches(it)) }
        val payload = json.getJSONObject("payload")
        val content = when (val kind = json.getString("kind")) {
            "forum.reply" -> AppNotificationContent.ForumReply(
                topicId = payload.getString("topic_id").also { require(UUID.matches(it)) },
                topicTitle = payload.getString("topic_title").take(MAX_TEXT),
                authorName = payload.getString("author_name").take(MAX_TEXT),
            )
            "contribution.validated", "contribution.rejected" -> AppNotificationContent.ContributionDecided(
                stationName = payload.getString("station_name").take(MAX_TEXT),
                fuel = WireFuelMapper.fromWire(payload.optString("fuel")).getOrNull(),
                validated = kind == "contribution.validated",
            )
            else -> null
        }
        if (content == null) null else AppNotification(
            id = id,
            source = AppNotificationSource.ACCOUNT,
            content = content,
            createdAtMillis = Instant.parse(json.getString("created_at")).toEpochMilli(),
            read = json.getBoolean("read"),
        )
    } catch (_: Exception) {
        null
    }

    private val UUID = Regex("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")
}
