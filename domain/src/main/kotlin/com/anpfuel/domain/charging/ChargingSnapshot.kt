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

private fun validDecimal(value: String, min: BigDecimal, max: BigDecimal): Boolean = try {
    val parsed = BigDecimal(value)
    parsed >= min && parsed <= max && parsed.stripTrailingZeros().scale() <= 12
} catch (_: Exception) {
    false
}

/** Closed wire vocabularies. Future values decode as UNKNOWN with the raw
 * wording retained beside them, never dropped or crashed on. */
enum class ChargingConnector { CCS2, TYPE2, CHADEMO, NACS, UNKNOWN }
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
) {
    init {
        require(canonicalId.matches(id))
        require(entityKeyPattern.matches(entityKey))
        require(externalId.isNotBlank() && externalId.length <= 256)
        require(rawConnectorCode.isNotBlank() && rawConnectorCode.length <= 128)
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
) {
    init {
        require(canonicalId.matches(id))
        require(entityKeyPattern.matches(entityKey))
        require(externalId.isNotBlank() && externalId.length <= 256)
        require(name.isNotBlank() && name.length <= 512)
        require(country == "BR")
        require(latitude == null || validDecimal(latitude, BigDecimal("-90"), BigDecimal("90")))
        require(longitude == null || validDecimal(longitude, BigDecimal("-180"), BigDecimal("180")))
        require(reportedPoints >= 0)
        require(groups.map { it.id }.distinct().size == groups.size)
        require(sources.size <= 100 && sources.distinctBy { it.source to it.externalId }.size == sources.size)
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

/** Stored batch counts with identity. Counts only; never a freshness claim. */
data class ChargingCoverage(
    val profile: String,
    val sourceRevision: String,
    val places: Long,
    val groups: Long,
    val vehicles: Long,
    val generatedAt: String,
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
