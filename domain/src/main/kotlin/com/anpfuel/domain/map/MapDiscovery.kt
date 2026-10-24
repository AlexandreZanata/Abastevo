package com.anpfuel.domain.map

import com.anpfuel.domain.valueobject.FuelProduct
import java.time.Instant
import java.time.LocalDate
import java.time.format.DateTimeParseException
import java.util.UUID

/** Frozen MAP-03 public overlay values. No storage, network or native types. */
enum class MapSource(val wire: String) {
    ANP("ANP"),
    COMMUNITY("COMMUNITY"),
    OSM("OSM"),
    OCM("OCM"),
    ;

    companion object {
        fun fromWire(value: String): MapSource =
            entries.firstOrNull { it.wire == value } ?: throw MapContractException("unknown-source")
    }
}

enum class MapService(val wire: String) {
    FUEL("FUEL"),
    CHARGING("CHARGING"),
    ;

    companion object {
        fun fromWire(value: String): MapService =
            entries.firstOrNull { it.wire == value } ?: throw MapContractException("unknown-service")
    }
}

enum class MapLocationQuality(val wire: String) {
    REVIEWED("reviewed"),
    UNKNOWN("unknown"),
    CITY_CENTROID("city-centroid"),
    ;

    companion object {
        fun fromWire(value: String): MapLocationQuality =
            entries.firstOrNull { it.wire == value } ?: throw MapContractException("unknown-quality")
    }
}

enum class MapDisplayPoint(val wire: String) {
    STATION("station"),
    ENTRANCE("entrance"),
    CITY_CENTROID("city_centroid"),
    ;

    companion object {
        fun fromWire(value: String): MapDisplayPoint =
            entries.firstOrNull { it.wire == value } ?: throw MapContractException("unknown-display-point")
    }
}

enum class MapPriceUnit(val wire: String) {
    L("L"),
    M3("M3"),
    KG_13("KG_13"),
}

enum class MapProduct(val wire: String, val fuelProduct: FuelProduct, val unit: MapPriceUnit) {
    ETHANOL_HYDRATED("ETHANOL_HYDRATED", FuelProduct.ETHANOL, MapPriceUnit.L),
    GASOLINE_REGULAR("GASOLINE_REGULAR", FuelProduct.GASOLINE_REGULAR, MapPriceUnit.L),
    GASOLINE_ADDITIVED("GASOLINE_ADDITIVED", FuelProduct.GASOLINE_PREMIUM, MapPriceUnit.L),
    GASOLINE_PREMIUM_GRADE("GASOLINE_PREMIUM_GRADE", FuelProduct.GASOLINE_PREMIUM_GRADE, MapPriceUnit.L),
    DIESEL_S500("DIESEL_S500", FuelProduct.DIESEL_S500, MapPriceUnit.L),
    DIESEL_S10("DIESEL_S10", FuelProduct.DIESEL_S10, MapPriceUnit.L),
    CNG("CNG", FuelProduct.CNG, MapPriceUnit.M3),
    LPG_P13("LPG_P13", FuelProduct.LPG_P13, MapPriceUnit.KG_13),
    ;

    companion object {
        fun fromWire(value: String): MapProduct =
            entries.firstOrNull { it.wire == value } ?: throw MapContractException("unknown-product")
    }
}

class MapContractException(val code: String) : IllegalArgumentException(code)

private val uuidPattern =
    Regex("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")
private val cnpjPattern = Regex("^[0-9A-Za-z]{14}$")
private val sha256Pattern = Regex("^[0-9a-f]{64}$")
private val hex40Pattern = Regex("^[0-9a-f]{40}$")

private fun requireUuid(value: String, code: String = "invalid-id"): String {
    if (!uuidPattern.matches(value)) throw MapContractException(code)
    return value
}

private fun requireDate(value: String, code: String): LocalDate =
    try {
        LocalDate.parse(value)
    } catch (_: DateTimeParseException) {
        throw MapContractException(code)
    }

private fun requireInstant(value: String, code: String): Instant =
    try {
        Instant.parse(value)
    } catch (_: DateTimeParseException) {
        throw MapContractException(code)
    }

data class MapIdentities(
    val stationId: String?,
    val cnpj: String?,
    val placeId: String?,
    val equipmentId: String?,
    val connectorId: String?,
    val osmType: String?,
    val osmId: Long?,
) {
    init {
        listOf(stationId, placeId, equipmentId, connectorId).filterNotNull()
            .forEach { requireUuid(it, "invalid-identity") }
        if (cnpj != null && !cnpjPattern.matches(cnpj)) throw MapContractException("invalid-cnpj")
        if ((osmType == null) != (osmId == null)) throw MapContractException("invalid-osm-identity")
        if (osmType != null && osmType !in setOf("node", "way", "relation")) {
            throw MapContractException("invalid-osm-type")
        }
        if (osmId != null && osmId < 1) throw MapContractException("invalid-osm-id")
    }
}

data class MapLocation(
    val latitudeDeg: String?,
    val longitudeDeg: String?,
    val quality: MapLocationQuality,
    val displayPoint: MapDisplayPoint?,
    val entranceLatitudeDeg: String?,
    val entranceLongitudeDeg: String?,
) {
    init {
        if ((latitudeDeg == null) != (longitudeDeg == null)) {
            throw MapContractException("incomplete-coordinate")
        }
        if (latitudeDeg != null) {
            val lat = latitudeDeg.toDoubleOrNull() ?: throw MapContractException("invalid-coordinate")
            val lon = longitudeDeg?.toDoubleOrNull() ?: throw MapContractException("invalid-coordinate")
            if (lat !in -90.0..90.0 || lon !in -180.0..180.0) throw MapContractException("invalid-coordinate")
        }
        when (quality) {
            MapLocationQuality.REVIEWED -> {
                if (latitudeDeg == null || displayPoint !in setOf(MapDisplayPoint.STATION, MapDisplayPoint.ENTRANCE)) {
                    throw MapContractException("invalid-reviewed-location")
                }
            }
            MapLocationQuality.UNKNOWN -> {
                if (latitudeDeg != null || displayPoint != null) throw MapContractException("fabricated-location")
            }
            MapLocationQuality.CITY_CENTROID -> {
                if (displayPoint != MapDisplayPoint.CITY_CENTROID) {
                    throw MapContractException("centroid-as-pin")
                }
            }
        }
        if ((entranceLatitudeDeg == null) != (entranceLongitudeDeg == null)) {
            throw MapContractException("incomplete-entrance")
        }
        if (entranceLatitudeDeg != null) {
            if (quality != MapLocationQuality.REVIEWED) throw MapContractException("entrance-without-review")
            val lat = entranceLatitudeDeg.toDoubleOrNull() ?: throw MapContractException("invalid-entrance")
            val lon = entranceLongitudeDeg?.toDoubleOrNull() ?: throw MapContractException("invalid-entrance")
            if (lat !in -90.0..90.0 || lon !in -180.0..180.0) throw MapContractException("invalid-entrance")
        }
    }

    val pinEligible: Boolean
        get() = quality == MapLocationQuality.REVIEWED &&
            latitudeDeg != null &&
            displayPoint in setOf(MapDisplayPoint.STATION, MapDisplayPoint.ENTRANCE)

    val entranceKnown: Boolean
        get() = entranceLatitudeDeg != null && entranceLongitudeDeg != null
}

data class MapPrice(
    val source: MapSource,
    val stationId: String,
    val product: MapProduct,
    val unit: MapPriceUnit,
    val amountMilliBrl: Long,
    val observedAt: LocalDate,
    val periodStart: LocalDate?,
    val periodEnd: LocalDate?,
) {
    init {
        if (source != MapSource.ANP && source != MapSource.COMMUNITY) {
            throw MapContractException("invalid-price-source")
        }
        requireUuid(stationId, "invalid-price-station")
        if (unit != product.unit) throw MapContractException("product-unit-mismatch")
        if (amountMilliBrl !in 1..1_000_000) throw MapContractException("amount-out-of-range")
        if ((periodStart == null) != (periodEnd == null)) throw MapContractException("incomplete-period")
        if (source == MapSource.ANP && periodStart == null) throw MapContractException("anp-period-required")
        if (source == MapSource.COMMUNITY && periodStart != null) throw MapContractException("community-period-forbidden")
        if (periodStart != null && periodEnd != null && periodStart.isAfter(periodEnd)) {
            throw MapContractException("invalid-period")
        }
    }
}

data class MapCharging(
    val placeId: String,
    val equipmentId: String,
    val connectorId: String,
    val connectorCode: String,
    val powerW: Long?,
    val access: String,
    val status: String,
    val tariffMilliBrlPerKwh: Long?,
    val tariffCondition: String?,
    val observedAt: Instant,
) {
    init {
        requireUuid(placeId, "invalid-charging-place")
        requireUuid(equipmentId, "invalid-charging-equipment")
        requireUuid(connectorId, "invalid-charging-connector")
        if (connectorCode.isEmpty() || connectorCode.length > 64) throw MapContractException("invalid-connector")
        if (powerW != null && powerW !in 0..1_000_000_000) throw MapContractException("invalid-power")
        if (access !in setOf("public", "private", "unknown")) throw MapContractException("invalid-access")
        if (status !in setOf("unknown", "available", "unavailable")) throw MapContractException("invalid-status")
        if (tariffMilliBrlPerKwh != null && tariffMilliBrlPerKwh !in 1..1_000_000_000) {
            throw MapContractException("invalid-tariff")
        }
        if (tariffCondition != null && tariffCondition !in setOf("STANDARD", "MEMBER", "SESSION", "PARKING")) {
            throw MapContractException("invalid-tariff-condition")
        }
        if (tariffMilliBrlPerKwh == null && tariffCondition != null) {
            throw MapContractException("condition-without-tariff")
        }
    }
}

data class MapClocks(val observedAt: Instant, val publishedAt: Instant, val fetchedAt: Instant)

data class MapRights(
    val source: MapSource,
    val license: String,
    val attribution: String,
    val sourceSha256: String,
) {
    init {
        if (license != "ADMITTED") throw MapContractException("rights-not-admitted")
        if (attribution.isBlank() || attribution.length > 500) throw MapContractException("invalid-attribution")
        if (!sha256Pattern.matches(sourceSha256)) throw MapContractException("invalid-source-hash")
    }
}

data class MapCapabilities(
    val priceKnown: Boolean,
    val pinEligible: Boolean,
    val entranceKnown: Boolean,
    val liveStatus: Boolean,
)

data class MapAssertion(
    val id: String,
    val source: MapSource,
    val kind: String,
    val name: String,
    val identities: MapIdentities,
    val location: MapLocation,
    val services: Set<MapService>,
    val prices: List<MapPrice>,
    val charging: MapCharging?,
    val clocks: MapClocks,
    val rights: MapRights,
    val capabilities: MapCapabilities,
) {
    init {
        requireUuid(id)
        if (kind !in setOf("FUEL_STATION", "CHARGING_SITE", "COMBINED_SITE", "REFERENCE_POI")) {
            throw MapContractException("unknown-kind")
        }
        if (name.isBlank() || name.length > 200) throw MapContractException("invalid-name")
        if (source == MapSource.ANP || source == MapSource.COMMUNITY) {
            if (identities.stationId == null) throw MapContractException("station-identity-required")
        }
        if (services.isEmpty()) throw MapContractException("service-required")
        if (prices.any { it.source != source || it.stationId != identities.stationId }) {
            throw MapContractException("price-identity-mismatch")
        }
        if (prices.map { "${it.source}|${it.product}|${it.unit}" }.toSet().size != prices.size) {
            throw MapContractException("duplicate-price")
        }
        when {
            charging != null && source !in setOf(MapSource.OCM, MapSource.COMMUNITY) ->
                throw MapContractException("charging-source-refused")
            charging == null && source == MapSource.OCM && MapService.CHARGING in services ->
                throw MapContractException("charging-facts-required")
        }
        if (charging != null) {
            if (identities.placeId != null && identities.placeId != charging.placeId) {
                throw MapContractException("charging-identity-mismatch")
            }
            if (identities.equipmentId != null && identities.equipmentId != charging.equipmentId) {
                throw MapContractException("charging-identity-mismatch")
            }
            if (identities.connectorId != null && identities.connectorId != charging.connectorId) {
                throw MapContractException("charging-identity-mismatch")
            }
        }
        if (rights.source != source) throw MapContractException("rights-source-mismatch")
        if (capabilities.priceKnown != prices.isNotEmpty()) throw MapContractException("price-flag-mismatch")
        if (capabilities.pinEligible != location.pinEligible) throw MapContractException("pin-flag-mismatch")
        if (capabilities.entranceKnown != location.entranceKnown) throw MapContractException("entrance-flag-mismatch")
        if (capabilities.liveStatus) throw MapContractException("live-status-not-admitted")
    }
}

data class MapAssertionCorpus(
    val format: String,
    val version: Int,
    val generation: String,
    val assertions: List<MapAssertion>,
) {
    init {
        if (format != "abastevo.map-assertions-v1") throw MapContractException("unsupported-format")
        if (version != 1) throw MapContractException("unsupported-version")
        if (generation.isBlank() || generation.length > 128) throw MapContractException("invalid-generation")
        if (assertions.isEmpty() || assertions.size > 10_000) throw MapContractException("assertion-count")
        if (assertions.map { it.id }.toSet().size != assertions.size) {
            throw MapContractException("duplicate-assertion-id")
        }
    }
}

data class NativeMapPackage(
    val id: String,
    val role: String,
    val path: String,
    val bytes: Long,
    val sha256: String,
) {
    init {
        if (!Regex("^[a-z0-9-]{1,64}$").matches(id)) throw MapContractException("invalid-package-id")
        if (role !in setOf("REGION", "OVERVIEW", "SEARCH", "ROUTING", "STYLE", "FONT")) {
            throw MapContractException("invalid-package-role")
        }
        if (!Regex("^[A-Za-z0-9._/-]{1,160}$").matches(path) || ".." in path || path.startsWith("/")) {
            throw MapContractException("unsafe-package-path")
        }
        if (bytes !in 1..4_294_967_296L) throw MapContractException("invalid-package-bytes")
        if (!sha256Pattern.matches(sha256)) throw MapContractException("invalid-package-hash")
    }
}

data class NativeMapManifest(
    val generation: String,
    val engineName: String,
    val upstreamSha: String,
    val generatorSha: String,
    val formatVersion: String,
    val regionId: String,
    val packages: List<NativeMapPackage>,
    val dependencies: List<String>,
    val license: String,
    val attribution: String,
    val generatedAt: Instant,
) {
    init {
        if (generation.isBlank()) throw MapContractException("invalid-generation")
        if (engineName != "comaps") throw MapContractException("unsupported-engine")
        if (!hex40Pattern.matches(upstreamSha) || !hex40Pattern.matches(generatorSha)) {
            throw MapContractException("unpinned-engine")
        }
        if (formatVersion.isBlank()) throw MapContractException("invalid-format-version")
        if (!Regex("^[a-z0-9-]{1,64}$").matches(regionId)) throw MapContractException("invalid-region")
        if (packages.isEmpty()) throw MapContractException("packages-required")
        if (packages.map { it.id }.toSet().size != packages.size) throw MapContractException("duplicate-package")
        if (dependencies.toSet() != packages.map { it.id }.toSet() && !packages.map { it.id }.containsAll(dependencies)) {
            throw MapContractException("missing-dependency")
        }
        if (license !in setOf("ODbL-1.0", "Apache-2.0", "MIT", "CC-BY-4.0")) {
            throw MapContractException("invalid-license")
        }
        if (attribution.isBlank() || attribution.length > 500) throw MapContractException("invalid-attribution")
    }
}
