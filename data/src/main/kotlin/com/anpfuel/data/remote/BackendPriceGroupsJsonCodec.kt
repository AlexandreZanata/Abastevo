package com.anpfuel.data.remote

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.BackendOfficialSection
import com.anpfuel.domain.model.BackendPriceGroup
import com.anpfuel.domain.model.BackendPriceGroups
import org.json.JSONArray
import org.json.JSONObject

/**
 * P10-T02 backend `PriceGroups` JSON codec.
 *
 * Parses `GET /v1/stations/{id}/prices` (`items` + `generated_at`) into
 * [BackendPriceGroups]. Community must stay null until P04; a non-null
 * community is refused explicitly rather than silently dropped. Encoding
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
            item.put("community", JSONObject.NULL)
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
        if (!item.isNull("community")) {
            throw DomainException("community must stay null until P04")
        }
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
            community = null,
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
