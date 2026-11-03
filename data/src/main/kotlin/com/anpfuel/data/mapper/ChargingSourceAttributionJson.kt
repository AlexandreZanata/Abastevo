package com.anpfuel.data.mapper

import com.anpfuel.domain.charging.ChargingSourceAttribution
import org.json.JSONArray
import org.json.JSONObject

/** Strict shared representation for public attribution in transport and storage. */
internal object ChargingSourceAttributionJson {
    private val keys = setOf("source", "external_id", "revision", "provider_id", "license", "attribution")

    fun decode(array: JSONArray): List<ChargingSourceAttribution> {
        require(array.length() in 1..100)
        return List(array.length()) { index ->
            val row = array.getJSONObject(index)
            require(row.keys().asSequence().all { it in keys })
            val provider = if (row.has("provider_id")) {
                val value = row.get("provider_id")
                require(value is Int || value is Long)
                (value as Number).toLong()
            } else {
                null
            }
            ChargingSourceAttribution(
                source = row.text("source"),
                externalId = row.text("external_id"),
                revision = row.text("revision"),
                providerId = provider,
                license = row.text("license"),
                attribution = row.text("attribution"),
            )
        }
    }

    private fun JSONObject.text(key: String): String = get(key) as? String
        ?: throw IllegalArgumentException("invalid charging source field")
}
