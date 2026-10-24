package com.anpfuel.data.remote.charging

import com.anpfuel.domain.charging.ChargingAccess
import com.anpfuel.domain.charging.ChargingConnectionGroup
import com.anpfuel.domain.charging.ChargingCoverage
import com.anpfuel.domain.charging.ChargingPlace
import com.anpfuel.domain.charging.ChargingPlacePage
import com.anpfuel.domain.charging.wireAccess
import com.anpfuel.domain.charging.wireConnector
import com.anpfuel.domain.charging.wireCurrent
import java.io.IOException
import org.json.JSONArray
import org.json.JSONObject

/** Typed transport failure: corrupt, oversize, foreign or private bodies. */
class ChargingSnapshotUnavailable(message: String? = null, cause: Throwable? = null) : IOException(message, cause)

/** Strict wire decode for EE-09 public charging reads (anonymous only).
 * Unknown connector/current/access values decode as UNKNOWN with raw
 * wording retained; corrupt, oversize (>100 rows), foreign or private
 * bodies fail closed and never reach callers or storage. */
object ChargingSnapshotJsonCodec {

    private const val MAX_ROWS = 100

    private val privateFields = setOf(
        "reporter", "price", "contributor", "contribution",
        "session_token", "access_token", "refresh_token", "family_id",
    )

    fun decodePlacePage(body: JSONObject): ChargingPlacePage {
        try {
            val items = body.getJSONArray("items")
            if (items.length() > MAX_ROWS) invalid("items over bound")
            val places = (0 until items.length()).map { decodePlace(items.getJSONObject(it)) }
            if (places.map { it.id }.distinct().size != places.size) invalid("duplicate place id")
            val next = optText(body, "next_cursor")
            if (next != null && next.isBlank()) invalid("blank cursor")
            return ChargingPlacePage(places, next, reqText(body, "generated_at"))
        } catch (e: ChargingSnapshotUnavailable) {
            throw e
        } catch (e: Exception) {
            throw ChargingSnapshotUnavailable("place page refused", e)
        }
    }

    fun decodePlaceDetail(body: JSONObject): ChargingPlace {
        try {
            val groups = body.getJSONArray("groups")
            if (groups.length() > MAX_ROWS) invalid("groups over bound")
            val decoded = (0 until groups.length()).map { decodeGroup(groups.getJSONObject(it)) }
            return decodePlace(body).copy(groups = decoded)
        } catch (e: ChargingSnapshotUnavailable) {
            throw e
        } catch (e: Exception) {
            throw ChargingSnapshotUnavailable("place detail refused", e)
        }
    }

    fun decodeCoverage(body: JSONObject): ChargingCoverage {
        try {
            return ChargingCoverage(
                profile = reqText(body, "profile"),
                sourceRevision = reqText(body, "source_revision"),
                places = reqCount(body, "places"),
                groups = reqCount(body, "groups"),
                vehicles = reqCount(body, "vehicles"),
                generatedAt = reqText(body, "generated_at"),
            )
        } catch (e: ChargingSnapshotUnavailable) {
            throw e
        } catch (e: Exception) {
            throw ChargingSnapshotUnavailable("coverage refused", e)
        }
    }

    private fun decodePlace(item: JSONObject): ChargingPlace {
        for (key in item.keys()) {
            if (key in privateFields) invalid("private field $key")
        }
        return ChargingPlace(
            id = reqText(item, "id"),
            entityKey = reqText(item, "entity_key"),
            externalId = reqText(item, "external_id"),
            name = reqText(item, "name"),
            country = reqText(item, "country"),
            latitude = optText(item, "latitude"),
            longitude = optText(item, "longitude"),
            publicAccess = wireAccess(reqText(item, "public_access")),
            reportedPoints = reqCount(item, "reported_points"),
            groups = emptyList(),
        )
    }

    private fun decodeGroup(item: JSONObject): ChargingConnectionGroup {
        for (key in item.keys()) {
            if (key in privateFields) invalid("private field $key")
        }
        return ChargingConnectionGroup(
            id = reqText(item, "id"),
            entityKey = reqText(item, "entity_key"),
            externalId = reqText(item, "external_id"),
            connector = wireConnector(reqText(item, "connector_code")),
            rawConnectorCode = reqText(item, "raw_connector_code"),
            current = wireCurrent(reqText(item, "current")),
            powerW = optCount(item, "power_w", minimum = 1),
            reportedQuantity = optCount(item, "reported_quantity", minimum = 1),
        )
    }

    private fun reqText(obj: JSONObject, key: String): String {
        if (obj.isNull(key)) invalid("missing $key")
        val value = obj.opt(key)
        if (value !is String || value.isEmpty() && key != "next_cursor") invalid("bad $key")
        return value as String
    }

    private fun optText(obj: JSONObject, key: String): String? {
        if (!obj.has(key) || obj.isNull(key)) return null
        val value = obj.opt(key)
        if (value !is String) invalid("bad $key")
        return value as String
    }

    private fun reqCount(obj: JSONObject, key: String): Long {
        if (obj.isNull(key)) invalid("missing $key")
        val value = obj.opt(key)
        if (value !is Number || value.toLong() < 0) invalid("bad $key")
        return value.toLong()
    }

    private fun optCount(obj: JSONObject, key: String, minimum: Long): Long? {
        if (!obj.has(key) || obj.isNull(key)) return null
        val value = obj.opt(key)
        if (value !is Number || value.toLong() < minimum) invalid("bad $key")
        return value.toLong()
    }

    private fun invalid(reason: String): Nothing = throw ChargingSnapshotUnavailable(reason)
}
