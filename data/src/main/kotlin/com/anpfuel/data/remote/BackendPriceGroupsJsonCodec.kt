package com.anpfuel.data.remote

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.BackendCommunitySection
import java.time.Instant
import com.anpfuel.domain.model.BackendOfficialSection
import com.anpfuel.domain.model.BackendPriceGroup
import com.anpfuel.domain.model.BackendPriceGroups
import org.json.JSONArray
import org.json.JSONObject

/**
 * P10-T02 backend `PriceGroups` JSON codec.
 *
 * Parses `GET /v1/stations/{id}/prices` (`items` + `generated_at`) into
 * [BackendPriceGroups]. Official and community sections are independent and typed. Encoding
 * preserves the same shape so Room round-trips stay lossless.
 */
object BackendPriceGroupsJsonCodec {

    fun decode(
        payload: String,
        stationId: String,
        fuelFilterWire: String?,
        fetchedAtMillis: Long,
        expiresAtMillis: Long,
    ): BackendPriceGroups {
        val groups = try {
            parseItems(payload)
        } catch (error: DomainException) {
            throw error
        } catch (error: Exception) {
            throw DomainException("malformed backend price payload", error)
        }
        return BackendPriceGroups.create(
            stationId = stationId,
            fuelFilterWire = fuelFilterWire,
            groups = groups,
            fetchedAtMillis = fetchedAtMillis,
            expiresAtMillis = expiresAtMillis,
        )
    }

    fun encode(groups: BackendPriceGroups): String {
        val items = JSONArray()
        for (group in groups.groups) {
            val item = JSONObject()
                .put("station_id", group.stationId)
                .put("fuel_product", group.fuelProductWire)
                .put("unit", group.unit)
                .put("condition", JSONObject().put("kind", group.conditionKind))
            val official = group.official
            if (official == null) {
                item.put("official", JSONObject.NULL)
            } else {
                item.put(
                    "official",
                    JSONObject()
                        .put("source", official.source)
                        .put("amount_milli_brl", official.amountMilliBrl)
                        .put("currency", official.currency)
                        .put("collected_on", official.collectedOn)
                        .put(
                            "survey_week",
                            JSONObject()
                                .put("start", official.surveyWeekStart)
                                .put("end", official.surveyWeekEnd),
                        )
                        .put("revision_id", official.revisionId),
                )
            }
            item.put("community", group.community?.let { c -> JSONObject()
                .put("source", c.source).put("availability", c.availability)
                .put("amount_milli_brl", c.amountMilliBrl ?: JSONObject.NULL).put("currency", c.currency)
                .put("confidence", c.confidence).put("freshness", c.freshness)
                .put("independent_supporters", c.supporters).put("confirmation_count", c.confirmations)
                .put("representative_observation_id", c.representativeObservationId ?: JSONObject.NULL)
                .put("last_observation_received_at", c.lastReceivedAt?.toString() ?: JSONObject.NULL)
                .put("expires_at", c.expiresAt?.toString() ?: JSONObject.NULL)
                .put("algorithm_version", c.algorithmVersion).put("projection_version", c.projectionVersion)
            } ?: JSONObject.NULL)
            items.put(item)
        }
        return JSONObject()
            .put("items", items)
            .put("generated_at", groups.fetchedAtMillis)
            .toString()
    }

    private fun parseItems(payload: String): List<BackendPriceGroup> {
        val doc = JSONObject(payload)
        val items = doc.optJSONArray("items") ?: JSONArray()
        val out = mutableListOf<BackendPriceGroup>()
        for (index in 0 until items.length()) {
            out += parseGroup(items.getJSONObject(index))
        }
        return out
    }

    private fun parseGroup(item: JSONObject): BackendPriceGroup {
        val official = if (item.isNull("official")) {
            null
        } else {
            parseOfficial(item.getJSONObject("official"))
        }
        val condition = item.optJSONObject("condition")
        return BackendPriceGroup.create(
            stationId = item.getString("station_id"),
            fuelProductWire = item.getString("fuel_product"),
            unit = item.optString("unit", ""),
            conditionKind = condition?.optString("kind", "") ?: "",
            official = official,
            community = if(item.isNull("community")) null else parseCommunity(item.getJSONObject("community")),
        )
    }

    private fun parseCommunity(doc: JSONObject): BackendCommunitySection {
        val amount = if (doc.isNull("amount_milli_brl")) null else doc.get("amount_milli_brl").let {
            require(it is Number && it.toString().matches(Regex("[0-9]+")))
            it.toString().toLong()
        }
        return BackendCommunitySection(
            doc.getString("source"), doc.getString("availability"), amount, doc.getString("currency"),
            doc.getString("confidence"), doc.getString("freshness"), doc.getInt("independent_supporters"),
            doc.getInt("confirmation_count"), if(doc.isNull("representative_observation_id")) null else doc.getString("representative_observation_id"),
            if(doc.isNull("last_observation_received_at")) null else Instant.parse(doc.getString("last_observation_received_at")),
            if(doc.isNull("expires_at")) null else Instant.parse(doc.getString("expires_at")),
            doc.getString("algorithm_version"), doc.getLong("projection_version"),
        )
    }

    private fun parseOfficial(doc: JSONObject): BackendOfficialSection {
        val week = doc.optJSONObject("survey_week")
        return BackendOfficialSection.create(
            source = doc.getString("source"),
            amountMilliBrl = doc.getLong("amount_milli_brl"),
            currency = doc.getString("currency"),
            collectedOn = doc.getString("collected_on"),
            surveyWeekStart = week?.getString("start") ?: "",
            surveyWeekEnd = week?.getString("end") ?: "",
            revisionId = doc.getString("revision_id"),
        )
    }
}
