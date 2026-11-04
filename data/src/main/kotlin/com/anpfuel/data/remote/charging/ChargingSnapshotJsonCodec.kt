package com.anpfuel.data.remote.charging

import com.anpfuel.data.mapper.ChargingSourceAttributionJson
import com.anpfuel.domain.charging.ChargingCatalogConfidence
import com.anpfuel.domain.charging.ChargingCatalogOffer
import com.anpfuel.domain.charging.ChargingConnectionGroup
import com.anpfuel.domain.charging.ChargingCoverage
import com.anpfuel.domain.charging.ChargingPlace
import com.anpfuel.domain.charging.ChargingPlacePage
import com.anpfuel.domain.charging.wireAccess
import com.anpfuel.domain.charging.wireConnector
import com.anpfuel.domain.charging.wireCurrent
import com.anpfuel.domain.discovery.formatStationAddress
import java.io.IOException
import org.json.JSONArray
import org.json.JSONObject

/** Typed transport failure: corrupt, oversize, foreign or private bodies. */
class ChargingSnapshotUnavailable(
    message: String? = null,
    cause: Throwable? = null,
    val httpStatus: Int? = null,
) : IOException(message, cause)

/** Strict wire decode for EE-09 public charging reads (anonymous only).
 * Unknown connector/current/access values decode as UNKNOWN with raw
 * wording retained; corrupt, oversize (>100 rows), foreign or private
 * bodies fail closed and never reach callers or storage. */
object ChargingSnapshotJsonCodec {

    private const val MAX_ROWS = 100

    private val addressKeys = listOf("street", "number", "district", "municipio", "uf")

    private val privateFields = setOf(
        "reporter", "price", "contributor", "contribution",
        "session_token", "access_token", "refresh_token", "family_id",
    )

    fun decodePlacePage(body: JSONObject): ChargingPlacePage {
        try {
            val items = body.getJSONArray("items")
            if (items.length() > MAX_ROWS) invalid("items over bound")
            val places = (0 until items.length()).map {
                val row = items.getJSONObject(it)
                decodePlace(row, groupsRequired = row.has("confidence"))
            }
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
            return decodePlace(body, groupsRequired = true)
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
                offer = if (body.has("offer")) body.getJSONObject("offer").let {
                    ChargingCatalogOffer(reqText(it, "notice"), reqText(it, "data"))
                } else null,
            )
        } catch (e: ChargingSnapshotUnavailable) {
            throw e
        } catch (e: Exception) {
            throw ChargingSnapshotUnavailable("coverage refused", e)
        }
    }

    private fun decodePlace(item: JSONObject, groupsRequired: Boolean): ChargingPlace {
        for (key in item.keys()) {
            if (key in privateFields) invalid("private field $key")
        }
        return ChargingPlace(
            id = reqText(item, "id"),
            entityKey = reqText(item, "entity_key"),
            externalId = reqText(item, "external_id"),
            name = optText(item, "name") ?: invalid("missing name"),
            country = reqText(item, "country"),
            latitude = optText(item, "latitude"),
            longitude = optText(item, "longitude"),
            publicAccess = wireAccess(reqText(item, "public_access")),
            reportedPoints = reqCount(item, "reported_points"),
            groups = if (groupsRequired) {
                val groups = item.getJSONArray("groups")
                if (groups.length() > MAX_ROWS) invalid("groups over bound")
                (0 until groups.length()).map { decodeGroup(groups.getJSONObject(it)) }
            } else emptyList(),
            catalogConfidence = when (optText(item, "confidence")) {
                null -> null
                "single-source" -> ChargingCatalogConfidence.SINGLE_SOURCE
                "two-source" -> ChargingCatalogConfidence.TWO_SOURCE
                else -> invalid("bad confidence")
            },
            linkedStationId = optText(item, "linked_station_id"),
            operator = optText(item, "operator"),
            addressLine = if (item.has("address")) {
                val address = item.getJSONObject("address")
                val parts = addressKeys.mapNotNull { key ->
                    optText(address, key)?.also {
                        if (it.length > 256) invalid("address component over bound")
                    }?.let { key to it }
                }.toMap()
                formatStationAddress(parts).takeIf { it.isNotEmpty() }
            } else null,
            sources = if (item.has("sources")) ChargingSourceAttributionJson.decode(item.getJSONArray("sources")) else emptyList(),
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
            rawConnectorCode = optText(item, "raw_connector_code") ?: invalid("missing raw_connector_code"),
            sourceKey = optText(item, "source_key"),
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

    private fun reqCount(obj: JSONObject, key: String): Long =
        optCount(obj, key, minimum = 0) ?: invalid("missing $key")

    private fun optCount(obj: JSONObject, key: String, minimum: Long): Long? {
        if (obj.isNull(key)) return null
        val count = when (val value = obj.opt(key)) {
            is Int -> value.toLong()
            is Long -> value
            else -> invalid("bad $key")
        }
        if (count < minimum) invalid("bad $key")
        return count
    }

    private fun invalid(reason: String): Nothing = throw ChargingSnapshotUnavailable(reason)
}
