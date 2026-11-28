package com.anpfuel.application.usecase.location

import com.anpfuel.application.usecase.directory.GetNearbyServerStationsUseCase
import com.anpfuel.application.usecase.directory.NearbyServerStationsOutcome
import com.anpfuel.domain.community.FeedCity
import com.anpfuel.domain.discovery.DeviceProximityRule
import com.anpfuel.domain.discovery.NearbyServerStation
import com.anpfuel.domain.discovery.StationMunicipality
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.rule.MunicipalitySearchTextNormalizer
import com.anpfuel.domain.valueobject.DeviceLocation
import kotlinx.coroutines.CancellationException

/** Stations around one device fix, nearest first, and the city they name (if any). */
data class DeviceSurroundings(
    val stations: List<NearbyServerStation>,
    val city: FeedCity?,
) {
    /** B-BR-LOC-01 suggestion for the contribution station picker. */
    val preselectable: NearbyServerStation? get() = DeviceProximityRule.preselectable(stations)
}

sealed interface CityFollowOutcome {
    /** [previous] lets the person undo the switch. */
    data class Switched(val previous: PreferredLocation?, val city: FeedCity) : CityFollowOutcome
    data object AlreadyThere : CityFollowOutcome
    data object Unavailable : CityFollowOutcome
}

/**
 * B-BR-LOC-01..03 — a driver who travels from city to city gets the city they
 * are in without picking it. The fix goes only to the anonymous nearby read
 * (no third-party geocoder, nothing persisted); the city is the municipality
 * of the nearest station, looked up in the local IBGE catalog.
 */
class FollowDeviceCityUseCase(
    private val nearbyStations: GetNearbyServerStationsUseCase,
    private val catalog: MunicipalityCatalogRepository,
    private val locations: SelectLocationUseCase,
) {

    /** Null when the directory read is off or fails; an empty page when nothing is near. */
    suspend fun surroundings(location: DeviceLocation): DeviceSurroundings? {
        val outcome = nearbyStations(
            lat = location.latitude,
            lon = location.longitude,
            radiusMeters = DeviceProximityRule.LOOKUP_RADIUS_METERS,
            limit = DeviceProximityRule.LOOKUP_LIMIT,
        )
        val stations = (outcome as? NearbyServerStationsOutcome.Fresh)?.stations ?: return null
        val city = DeviceProximityRule.municipalityOf(stations)?.let { resolveCity(it) }
        return DeviceSurroundings(stations = stations.sortedBy { it.distanceMeters }, city = city)
    }

    /** Makes [city] the preferred city unless it already is. */
    suspend fun follow(city: FeedCity): CityFollowOutcome {
        return try {
            val previous = locations.getPreferredLocation()
            if (previous != null && isSameCity(previous, city)) return CityFollowOutcome.AlreadyThere
            locations.selectLocation(state = city.state, municipality = city.name)
            CityFollowOutcome.Switched(previous = previous, city = city)
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (_: Exception) {
            CityFollowOutcome.Unavailable
        }
    }

    /** Puts back the city chosen before an automatic switch; false when that fails. */
    suspend fun restore(previous: PreferredLocation): Boolean =
        try {
            locations.selectLocation(state = previous.state, municipality = previous.municipality)
            true
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (_: Exception) {
            false
        }

    private suspend fun resolveCity(municipality: StationMunicipality): FeedCity? {
        val entry = catalog.getCatalogMunicipalities(municipality.state)
            .firstOrNull { it.ibgeCode == municipality.ibgeCode }
            ?: return null
        return FeedCity(code = municipality.ibgeCode, state = municipality.state, name = entry.municipality)
    }

    private suspend fun isSameCity(previous: PreferredLocation, city: FeedCity): Boolean {
        if (previous.state != city.state) return false
        val code = catalog.findCatalogEntry(previous.state, previous.municipality)?.ibgeCode
        return if (code != null) {
            code == city.code
        } else {
            MunicipalitySearchTextNormalizer.normalize(previous.municipality) ==
                MunicipalitySearchTextNormalizer.normalize(city.name)
        }
    }
}
