package com.anpfuel.domain.discovery

import com.anpfuel.domain.valueobject.BrazilianState

/** Municipality named by a canonical station: IBGE code and UF. */
data class StationMunicipality(val ibgeCode: String, val state: BrazilianState)

/**
 * What one device fix says about where the driver is, read from the nearest
 * canonical stations of a single `/v1/stations/nearby` page. Pure; the fix
 * itself is never kept.
 *
 * - B-BR-LOC-01 [preselectable]: the nearest station with a real map pin no
 *   farther than [PRESELECT_MAX_METERS]. It is a suggestion the contributor
 *   can reject, never a proximity claim or a trust signal.
 * - B-BR-LOC-02 [municipalityOf]: the municipality of the nearest station no
 *   farther than [CITY_MAX_METERS] that names a valid IBGE code and UF.
 */
object DeviceProximityRule {
    /** A driver photographing a board stands at the station; GPS and pin error stay inside this. */
    const val PRESELECT_MAX_METERS = 250.0

    /** The nearest station names the city only while it is reasonably close. */
    const val CITY_MAX_METERS = 15_000.0

    /** One bounded lookup serves both rules (the directory accepts 100..15000 m). */
    const val LOOKUP_RADIUS_METERS = 15_000
    const val LOOKUP_LIMIT = 10

    private val IBGE_CODE = Regex("^[0-9]{7}$")

    fun preselectable(stations: List<NearbyServerStation>): NearbyServerStation? =
        stations
            .filter { it.station.hasPin && it.distanceMeters.isFinite() && it.distanceMeters in 0.0..PRESELECT_MAX_METERS }
            .minByOrNull { it.distanceMeters }

    fun municipalityOf(stations: List<NearbyServerStation>): StationMunicipality? =
        stations
            .filter { it.distanceMeters.isFinite() && it.distanceMeters in 0.0..CITY_MAX_METERS }
            .sortedBy { it.distanceMeters }
            .firstNotNullOfOrNull { candidate ->
                val code = candidate.station.municipalityCode?.takeIf(IBGE_CODE::matches)
                val state = candidate.station.state?.let(BrazilianState::fromAbbreviation)
                if (code != null && state != null) StationMunicipality(code, state) else null
            }
}
