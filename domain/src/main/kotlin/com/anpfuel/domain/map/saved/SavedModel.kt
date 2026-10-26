package com.anpfuel.domain.map.saved

import java.math.BigDecimal
import java.math.RoundingMode
import java.security.MessageDigest

/** MAP-25 B-BR-MAP25-02 limits. Quotas refuse; they never evict private data. */
object SavedBounds {
    const val MAX_PLACES = 5_000
    const val MAX_TRIPS = 500
    const val MAX_POINTS_PER_TRIP = 10_000
    const val MAX_ROUTE_PLAN_POINTS = 102
    const val MAX_NAME = 120
    const val MAX_NOTE = 500
    const val MAX_SOURCE_ID = 128
    const val MAX_FILE_BYTES = 5 * 1024 * 1024
    const val MAX_ARCHIVE_ENTRIES = 64
    const val MAX_EXPANDED_BYTES = 20 * 1024 * 1024
    const val MAX_XML_DEPTH = 64
    const val MAX_XML_ELEMENTS = 200_000
    const val MAX_IMPORT_RECORDS = 5_000
}

class SavedValidationException(val code: String) : IllegalArgumentException(code)

enum class SavedSourceKind { ANP, COMMUNITY, MANUAL, IMPORT }
enum class SavedPlaceKind { STATION, PLACE }
enum class SavedTripKind { ROUTE_PLAN, RECORDED_TRACK, IMPORTED_TRACK }

/** Exact micro-degree coordinate; invalid input is refused, never clamped. */
data class GeoPoint(val latMicro: Long, val lonMicro: Long) {
    init {
        if (latMicro !in -90_000_000L..90_000_000L || lonMicro !in -180_000_000L..180_000_000L) {
            throw SavedValidationException("coordinate-out-of-range")
        }
    }

    val latitude: Double get() = latMicro / 1_000_000.0
    val longitude: Double get() = lonMicro / 1_000_000.0

    companion object {
        /** Exact decimal text, at most 9 fractional digits, rounded half-up to micro-degrees. */
        fun parse(lat: String, lon: String): GeoPoint = GeoPoint(micro(lat), micro(lon))

        fun fromDouble(lat: Double, lon: Double): GeoPoint {
            if (!lat.isFinite() || !lon.isFinite()) throw SavedValidationException("coordinate-not-finite")
            fun micro(value: Double): Long = try {
                BigDecimal(value).setScale(6, RoundingMode.HALF_UP).movePointRight(6).longValueExact()
            } catch (_: ArithmeticException) {
                throw SavedValidationException("coordinate-out-of-range")
            }
            return GeoPoint(micro(lat), micro(lon))
        }

        private val decimal = Regex("[+-]?[0-9]{1,3}(?:\\.[0-9]{1,18})?")

        private fun micro(text: String): Long {
            val trimmed = text.trim()
            if (!decimal.matches(trimmed)) throw SavedValidationException("coordinate-malformed")
            return try {
                BigDecimal(trimmed).setScale(6, RoundingMode.HALF_UP).movePointRight(6).longValueExact()
            } catch (_: ArithmeticException) {
                throw SavedValidationException("coordinate-malformed")
            }
        }
    }
}

private val uuid = Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")
private fun requireUuid(value: String, code: String) {
    if (!uuid.matches(value)) throw SavedValidationException(code)
}

private fun cleanText(value: String, max: Int, code: String, allowBlank: Boolean = false): String {
    val text = value.trim()
    if ((!allowBlank && text.isEmpty()) || text.length > max) throw SavedValidationException(code)
    if (text.any { it.isISOControl() && it != '\n' && it != '\t' }) throw SavedValidationException(code)
    return text
}

data class SavedPlace(
    val id: String,
    val kind: SavedPlaceKind,
    val name: String,
    val point: GeoPoint,
    val stationId: String?,
    val sourceKind: SavedSourceKind,
    val sourceId: String?,
    val note: String?,
    val createdAtMillis: Long,
) {
    init {
        requireUuid(id, "place-id-invalid")
        cleanText(name, SavedBounds.MAX_NAME, "place-name-invalid")
        when (kind) {
            SavedPlaceKind.STATION -> requireUuid(stationId ?: "", "station-id-required")
            SavedPlaceKind.PLACE -> if (stationId != null) throw SavedValidationException("station-id-forbidden")
        }
        if (sourceId != null) cleanText(sourceId, SavedBounds.MAX_SOURCE_ID, "source-id-invalid")
        if (note != null) cleanText(note, SavedBounds.MAX_NOTE, "note-invalid", allowBlank = true)
        if (createdAtMillis < 0) throw SavedValidationException("time-invalid")
    }
}

data class TripPoint(val point: GeoPoint, val elevationCm: Int? = null, val timeMillis: Long? = null) {
    init {
        if (elevationCm != null && elevationCm !in -1_000_000..10_000_000) throw SavedValidationException("elevation-invalid")
        if (timeMillis != null && timeMillis < 0) throw SavedValidationException("time-invalid")
    }
}

data class SavedTrip(
    val id: String,
    val kind: SavedTripKind,
    val name: String,
    val points: List<TripPoint>,
    val avoid: Set<String> = emptySet(),
    val sourceId: String? = null,
    val note: String? = null,
    val createdAtMillis: Long,
) {
    init {
        requireUuid(id, "trip-id-invalid")
        cleanText(name, SavedBounds.MAX_NAME, "trip-name-invalid")
        when (kind) {
            SavedTripKind.ROUTE_PLAN -> {
                if (points.size !in 2..SavedBounds.MAX_ROUTE_PLAN_POINTS) throw SavedValidationException("route-plan-points-invalid")
                if (points.any { it.timeMillis != null || it.elevationCm != null }) throw SavedValidationException("route-plan-points-invalid")
                if (points.map { it.point }.distinct().size != points.size) throw SavedValidationException("route-plan-points-invalid")
            }
            else -> {
                if (points.size !in 1..SavedBounds.MAX_POINTS_PER_TRIP) throw SavedValidationException("track-points-invalid")
                if (avoid.isNotEmpty()) throw SavedValidationException("track-avoid-forbidden")
            }
        }
        if (avoid.any { it !in ROAD_AVOIDANCE }) throw SavedValidationException("avoid-invalid")
        if (sourceId != null) cleanText(sourceId, SavedBounds.MAX_SOURCE_ID, "source-id-invalid")
        if (note != null) cleanText(note, SavedBounds.MAX_NOTE, "note-invalid", allowBlank = true)
        if (createdAtMillis < 0) throw SavedValidationException("time-invalid")
    }

    private companion object {
        val ROAD_AVOIDANCE = setOf("TOLL", "MOTORWAY", "FERRY", "UNPAVED")
    }
}

/** Owner scope of every saved row: guest or account, per API origin. */
data class SavedOwner(val owner: String, val origin: String) {
    init {
        if (owner.isEmpty() || owner.length > 128 || origin.isEmpty() || origin.length > 256) {
            throw SavedValidationException("owner-invalid")
        }
    }

    companion object {
        const val GUEST = "guest"
    }
}

object SavedFingerprint {
    fun sha256(bytes: ByteArray): String =
        MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }
}
