package com.anpfuel.domain.discovery

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.BackendPriceGroup

/**
 * P35-T01 — Bounded canonical server station identity.
 *
 * Consumes `GET /v1/stations`, `/v1/stations/nearby` and
 * `GET /v1/stations/{station_id}` without rewriting legacy CNPJ/offline
 * rows. The platform UUID is the stable key; legacy full-CNPJ identities
 * are resolved explicitly by the caller, never silently rewritten.
 * Coordinates appear only when the server provides them; a missing pair
 * stays null (honest unknown), never a fabricated centroid. Alphanumeric
 * `cnpj_normalized` is preserved verbatim with leading zeros.
 */
enum class StationLocationQuality(val wire: String, val pinRank: Int) {
    UNKNOWN("unknown", 0),
    CITY_CENTROID("city-centroid", 0),

    /** ANP official point confirmed by an OSM fuel POI. */
    REVIEWED("reviewed", 4),

    /** Community-mapped OSM fuel POI matched by CNPJ tag or exact address. */
    OSM_MAPPED("osm-mapped", 3),

    /** ANP official point not confirmed by OSM (approximate). */
    ANP_REPORTED("anp-reported", 2),

    /** IBGE CNEFE address point with exact street and number (approximate). */
    ADDRESS_POINT("address-point", 1),
    ;

    /**
     * B-BR-MAP-PIN-TIERS-01: only the four pin tiers may become map pins;
     * `unknown` and `city-centroid` stay list-only.
     */
    val isPin: Boolean get() = pinRank > 0

    /** Pin tiers whose point is not confirmed by a mapped POI. */
    val isApproximatePin: Boolean get() = this == ANP_REPORTED || this == ADDRESS_POINT

    companion object {
        /**
         * Forward compatible: an unrecognised future wire value degrades to
         * [UNKNOWN] (never a pin) instead of failing the whole response.
         */
        fun fromWire(wire: String): StationLocationQuality =
            entries.firstOrNull { it.wire == wire } ?: UNKNOWN
    }
}

/**
 * Best station to center a viewport on: the highest pin tier with
 * coordinates, keeping list order among equal tiers. Null when none.
 */
fun List<ServerStation>.bestPinnedStation(): ServerStation? =
    filter { it.hasPin }.maxByOrNull { it.locationQuality.pinRank }

class ServerStation private constructor(
    val stationId: String,
    val displayName: String,
    val locationQuality: StationLocationQuality,
    val latitude: Double?,
    val longitude: Double?,
    val cnpjNormalized: String?,
    val municipalityCode: String?,
    val state: String?,
    val currentRevisionId: String?,
    val address: Map<String, String>,
) {
    /** True when this station may be drawn as a map pin and routed to. */
    val hasPin: Boolean get() = locationQuality.isPin && latitude != null && longitude != null

    val addressLabel: String get() = address.values.filter { it.isNotBlank() }.joinToString(", ")

    val artwork = com.anpfuel.domain.profile.StationArtwork.BETA_DEFAULT

    companion object {
        fun create(
            stationId: String,
            displayName: String,
            locationQuality: StationLocationQuality,
            latitude: Double?,
            longitude: Double?,
            cnpjNormalized: String?,
            municipalityCode: String?,
            state: String?,
            currentRevisionId: String?,
            address: Map<String, String> = emptyMap(),
        ): ServerStation {
            if (!BackendPriceGroup.isUuid(stationId)) {
                throw DomainException("malformed station_id")
            }
            val name = displayName.trim()
            if (name.isEmpty()) throw DomainException("display_name is blank")
            if ((latitude == null) != (longitude == null)) {
                throw DomainException("coordinates must be both present or both absent")
            }
            if (latitude != null && (latitude !in -90.0..90.0)) {
                throw DomainException("latitude out of range")
            }
            if (longitude != null && (longitude !in -180.0..180.0)) {
                throw DomainException("longitude out of range")
            }
            val cnpj = cnpjNormalized?.trim()?.takeIf { it.isNotEmpty() }
            if (cnpj != null && cnpj.length > 14 + 4) {
                // Allow numeric 14 plus bounded alphanumeric variants; reject blobs.
                throw DomainException("cnpj_normalized too long")
            }
            if (currentRevisionId != null && !BackendPriceGroup.isUuid(currentRevisionId)) {
                throw DomainException("malformed current_revision_id")
            }
            return ServerStation(
                stationId = stationId.lowercase(),
                displayName = name,
                locationQuality = locationQuality,
                latitude = latitude,
                longitude = longitude,
                cnpjNormalized = cnpj,
                municipalityCode = municipalityCode?.trim()?.takeIf { it.isNotEmpty() },
                state = state?.trim()?.takeIf { it.isNotEmpty() },
                currentRevisionId = currentRevisionId?.lowercase(),
                address = address.toMap(),
            )
        }
    }
}

data class ServerStationPage(
    val items: List<ServerStation>,
    val nextCursor: String?,
    val cachedNextOffset: Int? = null,
)

data class NearbyServerStation(
    val station: ServerStation,
    val distanceMeters: Double,
)
