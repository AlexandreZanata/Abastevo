package com.anpfuel.data.remote

import com.anpfuel.domain.discovery.NearbyServerStation
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.ServerStationPage
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.exception.DomainException
import org.json.JSONObject

/**
 * P35-T01 Directory `Station` JSON codec.
 *
 * Decodes `GET /v1/stations` (`StationList`), `GET /v1/stations/nearby`
 * (`NearbyResult`, never echoing the request point) and
 * `GET /v1/stations/{station_id}` (`Station`). Malformed UUID/coordinates
 * are refused via [ServerStation]; no centroid is fabricated for unknown
 * locations.
 */
object DirectoryStationJsonCodec {

    /** Server pages never exceed the 100-item limit; cursors stay tiny like the feed. */
    const val MAX_PAGE_ITEMS = 100
    const val MAX_CURSOR_CHARS = 4096

    fun decodeStation(payload: String): ServerStation =
        try {
            parseStation(JSONObject(payload))
        } catch (error: DomainException) {
            throw error
        } catch (error: Exception) {
            throw DomainException("malformed station payload", error)
        }

    fun decodeList(payload: String): ServerStationPage =
        try {
            val doc = JSONObject(payload)
            val items = doc.getJSONArray("items")
            require(items.length() <= MAX_PAGE_ITEMS) { "page exceeds $MAX_PAGE_ITEMS items" }
            val out = mutableListOf<ServerStation>()
            for (index in 0 until items.length()) {
                out += parseStation(items.getJSONObject(index))
            }
            val cursor = optNullableString(doc, "next_cursor")
            require(cursor == null || cursor.length <= MAX_CURSOR_CHARS) { "cursor exceeds $MAX_CURSOR_CHARS chars" }
            ServerStationPage(items = out, nextCursor = cursor)
        } catch (error: DomainException) {
            throw error
        } catch (error: Exception) {
            throw DomainException("malformed station list payload", error)
        }

    fun decodeNearby(payload: String): List<NearbyServerStation> =
        try {
            val doc = JSONObject(payload)
            val items = doc.optJSONArray("items")
            val out = mutableListOf<NearbyServerStation>()
            if (items != null) {
                require(items.length() <= MAX_PAGE_ITEMS) { "page exceeds $MAX_PAGE_ITEMS items" }
                for (index in 0 until items.length()) {
                    val item = items.getJSONObject(index)
                    val distance = item.getDouble("distance_m")
                    if (distance < 0) throw DomainException("negative distance_m")
                    out += NearbyServerStation(station = parseStation(item), distanceMeters = distance)
                }
            }
            out
        } catch (error: DomainException) {
            throw error
        } catch (error: Exception) {
            throw DomainException("malformed nearby payload", error)
        }

    private fun parseStation(item: JSONObject): ServerStation {
        val coords = if (item.isNull("coordinates")) {
            null to null
        } else {
            val node = item.getJSONObject("coordinates")
            node.getDouble("lat") to node.getDouble("lon")
        }
        return ServerStation.create(
            stationId = item.getString("station_id"),
            displayName = item.getString("display_name"),
            locationQuality = StationLocationQuality.fromWire(item.getString("location_quality")),
            latitude = coords.first,
            longitude = coords.second,
            cnpjNormalized = optNullableString(item, "cnpj_normalized"),
            municipalityCode = optNullableString(item, "municipality_code"),
            state = optNullableString(item, "state"),
            currentRevisionId = optNullableString(item, "current_revision_id"),
            address = item.optJSONObject("address")?.let { address ->
                address.keys().asSequence().mapNotNull { key ->
                    if (address.isNull(key)) null else key to address.optString(key)
                }.toMap()
            } ?: emptyMap(),
        )
    }

    private fun optNullableString(item: JSONObject, key: String): String? {
        if (item.isNull(key)) return null
        return item.optString(key, "").takeIf { it.isNotEmpty() }
    }
}
