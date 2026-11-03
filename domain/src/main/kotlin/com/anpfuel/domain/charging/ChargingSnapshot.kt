package com.anpfuel.domain.charging

import java.math.BigDecimal
import java.time.Instant

private val canonicalId = Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")
private val entityKeyPattern = Regex("[0-9a-f]{64}")
private val httpsOrigin = Regex("https://[^\\s/]+(?::\\d+)?")

private fun validInstant(value: String): Boolean = try {
    Instant.parse(value)
    true
} catch (_: Exception) {
    false
}

private fun validDecimal(value: String, min: BigDecimal, max: BigDecimal, maxScale: Int): Boolean = try {
    val parsed = BigDecimal(value)
    parsed >= min && parsed <= max && parsed.stripTrailingZeros().scale() <= maxScale
} catch (_: Exception) {
    false
}

/** Closed wire vocabularies. Future values decode as UNKNOWN with the raw
 * wording retained beside them, never dropped or crashed on. */
enum class ChargingConnector(val supportsCommunityReport: Boolean = true) {
    CCS2, TYPE2, CHADEMO, NACS, UNKNOWN,
    TYPE1(false), CCS1(false), GBT_AC(false), GBT_DC(false), NBR14136(false), TESLA_LEGACY(false),
}

enum class ChargingCatalogConfidence { SINGLE_SOURCE, TWO_SOURCE }
enum class ChargingCurrent { AC, DC, UNKNOWN }
enum class ChargingAccess { PUBLIC, UNKNOWN }

fun wireConnector(raw: String): ChargingConnector = try {
    ChargingConnector.valueOf(raw)
} catch (_: Exception) {
    ChargingConnector.UNKNOWN
}

fun wireCurrent(raw: String): ChargingCurrent = try {
    ChargingCurrent.valueOf(raw)
} catch (_: Exception) {
    ChargingCurrent.UNKNOWN
}

fun wireAccess(raw: String): ChargingAccess = try {
    ChargingAccess.valueOf(raw)
} catch (_: Exception) {
    ChargingAccess.UNKNOWN
}

/** One public connector group. Unknown connector/current values stay
 * UNKNOWN with raw wording; coordinates never appear here. */
data class ChargingConnectionGroup(
    val id: String,
    val entityKey: String,
    val externalId: String,
    val connector: ChargingConnector,
    val rawConnectorCode: String,
    val current: ChargingCurrent,
    val powerW: Long?,
    val reportedQuantity: Long?,
    val sourceKey: String? = null,
) {
    init {
        require(canonicalId.matches(id))
        require(entityKeyPattern.matches(entityKey))
        require(externalId.isNotBlank() && externalId.length <= 256)
        require(
            if (sourceKey == null) rawConnectorCode.isNotBlank() && rawConnectorCode.length <= 128
            else rawConnectorCode.length <= 256 && rawConnectorCode.none(Char::isISOControl)
        )
        require(powerW == null || powerW >= 1)
        require(reportedQuantity == null || reportedQuantity >= 1)
    }
}

/** Public attribution for one admitted source assertion. */
data class ChargingSourceAttribution(
    val source: String,
    val externalId: String,
    val revision: String,
    val providerId: Long?,
    val license: String,
    val attribution: String,
) {
    init {
        require(source == "OSM" || source == "OCM")
        require(if (source == "OCM") providerId != null && providerId > 0 else providerId == null)
        require(externalId.isNotBlank() && externalId.length <= 128 && externalId.none(Char::isISOControl))
        require(revision.isNotBlank() && revision.length <= 128 && revision.none(Char::isISOControl))
        require(license.isNotBlank() && license.length <= 512 && license.none(Char::isISOControl))
        require(attribution.isNotBlank() && attribution.length <= 512 && attribution.none(Char::isISOControl))
    }
}

/** One public charging location. Absent coordinates stay null (unknown);
 * no centroid is ever fabricated. No private fields cross this type. */
data class ChargingPlace(
    val id: String,
    val entityKey: String,
    val externalId: String,
    val name: String,
    val country: String,
    val latitude: String?,
    val longitude: String?,
    val publicAccess: ChargingAccess,
    val reportedPoints: Long,
    val groups: List<ChargingConnectionGroup>,
    val sources: List<ChargingSourceAttribution> = emptyList(),
    val catalogConfidence: ChargingCatalogConfidence? = null,
    val linkedStationId: String? = null,
) {
    val confirmedByTwoSources: Boolean
        get() = catalogConfidence == ChargingCatalogConfidence.TWO_SOURCE

    init {
        require(canonicalId.matches(id))
        require(entityKeyPattern.matches(entityKey))
        require(externalId.isNotBlank() && externalId.length <= 256)
        require(name.length <= 512 && (name.isNotBlank() || catalogConfidence != null && name.isEmpty()))
        require(linkedStationId == null || catalogConfidence != null && canonicalId.matches(linkedStationId))
        require(country == "BR")
        val coordinateScale = if (catalogConfidence == null) 12 else 16
        require(latitude == null || validDecimal(latitude, BigDecimal("-90"), BigDecimal("90"), coordinateScale))
        require(longitude == null || validDecimal(longitude, BigDecimal("-180"), BigDecimal("180"), coordinateScale))
        require(reportedPoints >= 0)
        require(groups.map { it.id }.distinct().size == groups.size)
        require(sources.size <= 100 && sources.distinctBy { it.source to it.externalId }.size == sources.size)
        if (catalogConfidence == null) {
            require(groups.none { it.sourceKey != null })
        } else {
            require(sources.isNotEmpty())
            if (confirmedByTwoSources) require(sources.map { it.source }.toSet() == setOf("OSM", "OCM"))
            val ownerKeys = sources.mapTo(hashSetOf()) {
                if (it.source == "OSM") "osm:${it.externalId}" else "ocm:poi:${it.externalId}"
            }
            require(groups.all { it.sourceKey in ownerKeys })
        }
    }
}

data class ChargingPlacePage(
    val items: List<ChargingPlace>,
    val nextCursor: String?,
    val generatedAt: String,
) {
    init {
        require(items.map { it.id }.distinct().size == items.size)
        require(nextCursor == null || nextCursor.isNotBlank())
        require(validInstant(generatedAt))
    }
}

data class ChargingCatalogOffer(val noticePath: String, val dataPath: String) {
    companion object {
        fun forRevision(revision: String): ChargingCatalogOffer? {
            if (!entityKeyPattern.matches(revision)) return null
            val base = "/v1/charging/catalog/$revision/offer"
            return ChargingCatalogOffer("$base/derived-database-offer.json", "$base/derived-database.ndjson")
        }
    }
}

/** Stored batch counts with identity. Counts only; never a freshness claim. */
data class ChargingCoverage(
    val profile: String,
    val sourceRevision: String,
    val places: Long,
    val groups: Long,
    val vehicles: Long,
    val generatedAt: String,
    val offer: ChargingCatalogOffer? = null,
) {
    init {
        require(profile.isNotBlank() && sourceRevision.isNotBlank())
        require(places >= 0 && groups >= 0 && vehicles >= 0)
        require(validInstant(generatedAt))
    }
}

/** Pack manifest carried beside every snapshot generation: which source
 * profile and revision, from which origin, under which license note, in
 * which schema, on top of which base generation. The license note is the
 * caller-observed wording, never a grant invented here. */
data class ChargingPackMeta(
    val profile: String,
    val sourceRevision: String,
    val origin: String,
    val license: String,
    val schemaVersion: Int,
    val baseGeneration: Long,
    val generatedAt: String,
) {
    init {
        require(profile.isNotBlank() && sourceRevision.isNotBlank())
        require(httpsOrigin.matches(origin))
        require(license.isNotBlank() && license.length <= 280)
        require(schemaVersion == 1)
        require(baseGeneration >= 0)
        require(validInstant(generatedAt))
    }
}
