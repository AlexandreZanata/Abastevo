package com.anpfuel.data.mapper

import com.anpfuel.domain.map.MapAssertion
import com.anpfuel.domain.map.MapAssertionCorpus
import com.anpfuel.domain.map.MapCapabilities
import com.anpfuel.domain.map.MapCharging
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
import com.anpfuel.domain.map.NativeMapManifest
import com.anpfuel.domain.map.NativeMapPackage
import java.time.Instant
import java.time.LocalDate
import org.json.JSONArray
import org.json.JSONObject

/**
 * Strict MAP-03 wire decoder. Unknown keys, missing fields, drifted enums,
 * floats-as-money and duplicate identifiers fail the whole corpus instead
 * of producing a partial public overlay.
 */
object MapPoiCodec {

    fun decodeCorpus(raw: String): MapAssertionCorpus {
        val root = objectOf(raw)
        strict(root, "format", "version", "generation", "assertions")
        val assertions = root.getJSONArray("assertions")
        if (assertions.length() !in 1..10_000) throw MapContractException("assertion-count")
        return MapAssertionCorpus(
            format = root.getString("format"),
            version = root.getInt("version"),
            generation = root.getString("generation"),
            assertions = (0 until assertions.length()).map { decodeAssertion(assertions.getJSONObject(it)) },
        )
    }

    fun decodeManifest(raw: String): NativeMapManifest {
        val root = objectOf(raw)
        strict(
            root,
            "format", "version", "generation", "engine", "region", "packages",
            "dependencies", "sources", "rights", "generated_at",
        )
        if (root.getString("format") != "abastevo.native-map-manifest-v1" || root.getInt("version") != 1) {
            throw MapContractException("unsupported-manifest")
        }
        val engine = root.getJSONObject("engine")
        strict(engine, "name", "upstream_sha", "generator_sha", "format_version")
        val region = root.getJSONObject("region")
        strict(region, "id", "name", "bbox", "overview")
        val bbox = region.getJSONObject("bbox")
        strict(bbox, "min_lon_deg", "min_lat_deg", "max_lon_deg", "max_lat_deg")
        if (bbox.getDouble("min_lon_deg") >= bbox.getDouble("max_lon_deg") ||
            bbox.getDouble("min_lat_deg") >= bbox.getDouble("max_lat_deg")
        ) {
            throw MapContractException("invalid-bbox")
        }
        val packages = root.getJSONArray("packages")
        if (packages.length() !in 1..200) throw MapContractException("package-count")
        val decoded = (0 until packages.length()).map {
            val p = packages.getJSONObject(it)
            strict(p, "id", "role", "path", "bytes", "sha256")
            NativeMapPackage(
                id = p.getString("id"),
                role = p.getString("role"),
                path = p.getString("path"),
                bytes = p.getLong("bytes"),
                sha256 = p.getString("sha256"),
            )
        }
        val dependencies = strings(root.getJSONArray("dependencies"))
        if (dependencies.toSet().size != dependencies.size) throw MapContractException("duplicate-dependency")
        val ids = decoded.map { it.id }.toSet()
        if (!ids.containsAll(dependencies)) throw MapContractException("missing-dependency")
        val sources = root.getJSONArray("sources")
        if (sources.length() !in 1..50) throw MapContractException("source-count")
        val licenses = (0 until sources.length()).map {
            val s = sources.getJSONObject(it)
            strict(s, "id", "license", "attribution", "sha256")
            if (s.getString("attribution").isBlank() || !s.getString("sha256").matches(sha256)) {
                throw MapContractException("invalid-source")
            }
            s.getString("license")
        }.toSet()
        val rights = root.getJSONObject("rights")
        strict(rights, "license", "attribution", "commercial", "offline", "redistribution")
        if (!rights.getBoolean("commercial") || !rights.getBoolean("offline") || !rights.getBoolean("redistribution")) {
            throw MapContractException("rights-denied")
        }
        val license = rights.getString("license")
        if (license !in licenses) throw MapContractException("license-not-in-sources")
        return NativeMapManifest(
            generation = root.getString("generation"),
            engineName = engine.getString("name"),
            upstreamSha = engine.getString("upstream_sha"),
            generatorSha = engine.getString("generator_sha"),
            formatVersion = engine.getString("format_version"),
            regionId = region.getString("id"),
            packages = decoded,
            dependencies = dependencies,
            license = license,
            attribution = rights.getString("attribution"),
            generatedAt = Instant.parse(root.getString("generated_at")),
        )
    }

    private fun decodeAssertion(obj: JSONObject): MapAssertion {
        strict(
            obj,
            "id", "source", "kind", "name", "identities", "location", "services",
            "prices", "charging", "clocks", "rights", "capabilities",
        )
        val identities = obj.getJSONObject("identities")
        strict(
            identities,
            "station_uuid", "cnpj", "place_id", "equipment_id", "connector_id", "osm_type", "osm_id",
        )
        val location = obj.getJSONObject("location")
        strict(
            location,
            "latitude_deg", "longitude_deg", "quality", "display_point",
            "entrance_latitude_deg", "entrance_longitude_deg",
        )
        val services = obj.getJSONArray("services")
        val prices = obj.getJSONArray("prices")
        if (prices.length() > 20) throw MapContractException("price-count")
        val charging = if (obj.isNull("charging")) null else decodeCharging(obj.getJSONObject("charging"))
        val clocks = obj.getJSONObject("clocks")
        strict(clocks, "observed_at", "published_at", "fetched_at")
        val rights = obj.getJSONObject("rights")
        strict(rights, "source", "license", "attribution", "source_sha256")
        val capabilities = obj.getJSONObject("capabilities")
        strict(capabilities, "price_known", "pin_eligible", "entrance_known", "live_status")
        return MapAssertion(
            id = obj.getString("id"),
            source = MapSource.fromWire(obj.getString("source")),
            kind = obj.getString("kind"),
            name = obj.getString("name"),
            identities = MapIdentities(
                stationId = optString(identities, "station_uuid"),
                cnpj = optString(identities, "cnpj"),
                placeId = optString(identities, "place_id"),
                equipmentId = optString(identities, "equipment_id"),
                connectorId = optString(identities, "connector_id"),
                osmType = optString(identities, "osm_type"),
                osmId = optLong(identities, "osm_id"),
            ),
            location = MapLocation(
                latitudeDeg = optString(location, "latitude_deg"),
                longitudeDeg = optString(location, "longitude_deg"),
                quality = MapLocationQuality.fromWire(location.getString("quality")),
                displayPoint = optString(location, "display_point")?.let(MapDisplayPoint::fromWire),
                entranceLatitudeDeg = optString(location, "entrance_latitude_deg"),
                entranceLongitudeDeg = optString(location, "entrance_longitude_deg"),
            ),
            services = (0 until services.length()).map { MapService.fromWire(services.getString(it)) }.toSet(),
            prices = (0 until prices.length()).map { decodePrice(prices.getJSONObject(it)) },
            charging = charging,
            clocks = MapClocks(
                observedAt = Instant.parse(clocks.getString("observed_at")),
                publishedAt = Instant.parse(clocks.getString("published_at")),
                fetchedAt = Instant.parse(clocks.getString("fetched_at")),
            ),
            rights = MapRights(
                source = MapSource.fromWire(rights.getString("source")),
                license = rights.getString("license"),
                attribution = rights.getString("attribution"),
                sourceSha256 = rights.getString("source_sha256"),
            ),
            capabilities = MapCapabilities(
                priceKnown = capabilities.getBoolean("price_known"),
                pinEligible = capabilities.getBoolean("pin_eligible"),
                entranceKnown = capabilities.getBoolean("entrance_known"),
                liveStatus = capabilities.getBoolean("live_status"),
            ),
        )
    }

    private fun decodePrice(obj: JSONObject): MapPrice {
        strict(
            obj,
            "source", "station_uuid", "product", "unit", "amount_milli_brl",
            "observed_at", "period_start", "period_end",
        )
        return MapPrice(
            source = MapSource.fromWire(obj.getString("source")),
            stationId = obj.getString("station_uuid"),
            product = MapProduct.fromWire(obj.getString("product")),
            unit = MapPriceUnit.valueOf(obj.getString("unit")),
            amountMilliBrl = obj.getLong("amount_milli_brl"),
            observedAt = LocalDate.parse(obj.getString("observed_at")),
            periodStart = optString(obj, "period_start")?.let(LocalDate::parse),
            periodEnd = optString(obj, "period_end")?.let(LocalDate::parse),
        )
    }

    private fun decodeCharging(obj: JSONObject): MapCharging {
        strict(
            obj,
            "place_id", "equipment_id", "connector_id", "connector_code", "power_w",
            "access", "status", "tariff_milli_brl_per_kwh", "tariff_condition", "observed_at",
        )
        return MapCharging(
            placeId = obj.getString("place_id"),
            equipmentId = obj.getString("equipment_id"),
            connectorId = obj.getString("connector_id"),
            connectorCode = obj.getString("connector_code"),
            powerW = optLong(obj, "power_w"),
            access = obj.getString("access"),
            status = obj.getString("status"),
            tariffMilliBrlPerKwh = optLong(obj, "tariff_milli_brl_per_kwh"),
            tariffCondition = optString(obj, "tariff_condition"),
            observedAt = Instant.parse(obj.getString("observed_at")),
        )
    }

    private fun objectOf(raw: String): JSONObject =
        try {
            JSONObject(raw)
        } catch (_: Exception) {
            throw MapContractException("invalid-json")
        }

    private fun strict(obj: JSONObject, vararg keys: String) {
        val actual = obj.keys().asSequence().toSet()
        if (actual != keys.toSet()) throw MapContractException("unknown-or-missing-field")
    }

    private fun optString(obj: JSONObject, key: String): String? =
        if (obj.isNull(key)) null else obj.getString(key)

    private fun optLong(obj: JSONObject, key: String): Long? =
        if (obj.isNull(key)) null else obj.getLong(key)

    private fun strings(array: JSONArray): List<String> =
        (0 until array.length()).map { array.getString(it) }

    private val sha256 = Regex("^[0-9a-f]{64}$")
}
