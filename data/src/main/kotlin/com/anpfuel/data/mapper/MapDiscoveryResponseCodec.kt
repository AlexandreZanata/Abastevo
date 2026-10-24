package com.anpfuel.data.mapper

import com.anpfuel.domain.map.MapAssertion
import com.anpfuel.domain.map.MapCapabilities
import com.anpfuel.domain.map.MapClocks
import com.anpfuel.domain.map.MapContractException
import com.anpfuel.domain.map.MapDisplayPoint
import com.anpfuel.domain.map.MapIdentities
import com.anpfuel.domain.map.MapLocation
import com.anpfuel.domain.map.MapLocationQuality
import com.anpfuel.domain.map.MapPrice
import com.anpfuel.domain.map.MapPriceUnit
import com.anpfuel.domain.map.MapProduct
import com.anpfuel.domain.map.MapRights
import com.anpfuel.domain.map.MapService
import com.anpfuel.domain.map.MapSource
import java.time.Instant
import java.time.LocalDate
import org.json.JSONArray
import org.json.JSONObject

data class MapPageGeneration(
    val id: String,
    val key: String,
    val sourceRevision: String,
    val status: String,
    val activatedAt: Instant,
)

data class MapAssertionPage(
    val generation: MapPageGeneration?,
    val items: List<MapAssertion>,
    val nextCursor: String?,
    val generatedAt: Instant,
)

/**
 * Strict decoder for the actual `/v1/map/assertions` HTTP response. It reuses
 * the frozen domain validators so API bytes preserve the same identity, price
 * and location rules as the prepared corpus.
 */
object MapDiscoveryResponseCodec {
    private val uuid = Regex("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")
    private val sha256 = Regex("^[0-9a-f]{64}$")

    fun decode(raw: String): MapAssertionPage {
        val root = MapPoiCodec.objectOf(raw)
        MapPoiCodec.strict(root, "generation", "items", "next_cursor", "generated_at")
        val generation = if (root.isNull("generation")) null else decodeGeneration(root.getJSONObject("generation"))
        val items = root.getJSONArray("items")
        if (items.length() > 100) throw MapContractException("item-count")
        val next = MapPoiCodec.optString(root, "next_cursor")
        if (next != null && (next.isBlank() || next.length > 2048)) throw MapContractException("invalid-cursor")
        return MapAssertionPage(
            generation = generation,
            items = (0 until items.length()).map { decodeItem(items.getJSONObject(it)) },
            nextCursor = next,
            generatedAt = Instant.parse(root.getString("generated_at")),
        )
    }

    private fun decodeGeneration(obj: JSONObject): MapPageGeneration {
        MapPoiCodec.strict(obj, "id", "key", "source_revision", "status", "activated_at")
        val id = obj.getString("id")
        if (!uuid.matches(id)) throw MapContractException("invalid-generation-id")
        val status = obj.getString("status")
        if (status !in setOf("ACTIVE", "SUPERSEDED", "REVOKED")) {
            throw MapContractException("invalid-generation-status")
        }
        return MapPageGeneration(
            id = id,
            key = obj.getString("key"),
            sourceRevision = obj.getString("source_revision"),
            status = status,
            activatedAt = Instant.parse(obj.getString("activated_at")),
        )
    }

    private fun decodeItem(obj: JSONObject): MapAssertion {
        val required = setOf(
            "assertion_id", "source", "kind", "name", "station_uuid", "cnpj", "place_id",
            "equipment_id", "connector_id", "osm_type", "osm_id", "location_quality",
            "display_point", "coordinates", "services", "prices", "clocks", "rights",
            "capabilities",
        )
        val optional = setOf("entrance", "charging")
        val actual = obj.keys().asSequence().toSet()
        if (!actual.containsAll(required) || !(required + optional).containsAll(actual)) {
            throw MapContractException("unknown-or-missing-field")
        }
        val id = obj.getString("assertion_id")
        if (!uuid.matches(id)) throw MapContractException("invalid-assertion-id")
        val stationId = MapPoiCodec.optString(obj, "station_uuid")
        val coordinates = if (obj.isNull("coordinates")) null else obj.getJSONObject("coordinates")
        coordinates?.let { MapPoiCodec.strict(it, "latitude_deg", "longitude_deg") }
        val entrance = if (!obj.has("entrance") || obj.isNull("entrance")) null else obj.getJSONObject("entrance")
        entrance?.let { MapPoiCodec.strict(it, "latitude_deg", "longitude_deg") }
        val services = obj.getJSONArray("services")
        val prices = obj.getJSONArray("prices")
        if (prices.length() > 20) throw MapContractException("price-count")
        val clocks = obj.getJSONObject("clocks")
        MapPoiCodec.strict(clocks, "observed_at", "published_at", "fetched_at")
        val rights = obj.getJSONObject("rights")
        MapPoiCodec.strict(rights, "source", "license", "attribution", "source_sha256")
        val capabilities = obj.getJSONObject("capabilities")
        MapPoiCodec.strict(capabilities, "price_known", "pin_eligible", "entrance_known", "live_status")
        return MapAssertion(
            id = id,
            source = MapSource.fromWire(obj.getString("source")),
            kind = obj.getString("kind"),
            name = obj.getString("name"),
            identities = MapIdentities(
                stationId = stationId,
                cnpj = MapPoiCodec.optString(obj, "cnpj"),
                placeId = MapPoiCodec.optString(obj, "place_id"),
                equipmentId = MapPoiCodec.optString(obj, "equipment_id"),
                connectorId = MapPoiCodec.optString(obj, "connector_id"),
                osmType = MapPoiCodec.optString(obj, "osm_type"),
                osmId = MapPoiCodec.optLong(obj, "osm_id"),
            ),
            location = MapLocation(
                latitudeDeg = coordinates?.let { MapPoiCodec.optString(it, "latitude_deg") },
                longitudeDeg = coordinates?.let { MapPoiCodec.optString(it, "longitude_deg") },
                quality = MapLocationQuality.fromWire(obj.getString("location_quality")),
                displayPoint = MapPoiCodec.optString(obj, "display_point")?.let(MapDisplayPoint::fromWire),
                entranceLatitudeDeg = entrance?.let { MapPoiCodec.optString(it, "latitude_deg") },
                entranceLongitudeDeg = entrance?.let { MapPoiCodec.optString(it, "longitude_deg") },
            ),
            services = (0 until services.length()).map { MapService.fromWire(services.getString(it)) }.toSet(),
            prices = (0 until prices.length()).map { decodePrice(prices.getJSONObject(it), stationId) },
            charging = if (!obj.has("charging") || obj.isNull("charging")) null else MapPoiCodec.decodeCharging(obj.getJSONObject("charging")),
            clocks = MapClocks(
                observedAt = Instant.parse(clocks.getString("observed_at")),
                publishedAt = Instant.parse(clocks.getString("published_at")),
                fetchedAt = Instant.parse(clocks.getString("fetched_at")),
            ),
            rights = MapRights(
                source = MapSource.fromWire(rights.getString("source")),
                license = rights.getString("license"),
                attribution = rights.getString("attribution"),
                sourceSha256 = rights.getString("source_sha256").also {
                    if (!sha256.matches(it)) throw MapContractException("invalid-source-hash")
                },
            ),
            capabilities = MapCapabilities(
                priceKnown = capabilities.getBoolean("price_known"),
                pinEligible = capabilities.getBoolean("pin_eligible"),
                entranceKnown = capabilities.getBoolean("entrance_known"),
                liveStatus = capabilities.getBoolean("live_status"),
            ),
        )
    }

    private fun decodePrice(obj: JSONObject, stationId: String?): MapPrice {
        MapPoiCodec.strict(obj, "source", "product", "unit", "amount_milli_brl", "observed_at", "period_start", "period_end")
        if (stationId == null) throw MapContractException("price-without-station")
        return MapPrice(
            source = MapSource.fromWire(obj.getString("source")),
            stationId = stationId,
            product = MapProduct.fromWire(obj.getString("product")),
            unit = MapPriceUnit.valueOf(obj.getString("unit")),
            amountMilliBrl = obj.getLong("amount_milli_brl"),
            observedAt = LocalDate.parse(obj.getString("observed_at")),
            periodStart = MapPoiCodec.optString(obj, "period_start")?.let(LocalDate::parse),
            periodEnd = MapPoiCodec.optString(obj, "period_end")?.let(LocalDate::parse),
        )
    }
}
