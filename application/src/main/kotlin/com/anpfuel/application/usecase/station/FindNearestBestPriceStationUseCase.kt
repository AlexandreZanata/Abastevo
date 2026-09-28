package com.anpfuel.application.usecase.station

import com.anpfuel.application.usecase.price.GetStationPricesUseCase
import com.anpfuel.application.usecase.price.StationPricesOutcome
import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.repository.AddressGeocodeOutcome
import com.anpfuel.domain.repository.AddressGeocodeRepository
import com.anpfuel.domain.repository.GeocodeRequest
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.NearestBestPriceStationRule
import com.anpfuel.domain.rule.StationAddressNormalizationRule
import com.anpfuel.domain.rule.StationAddressParsingRule
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.GeoCoordinates
import com.anpfuel.domain.valueobject.PriceAmount

sealed interface FindNearestStationOutcome {
    data class Success(
        val station: RetailStation,
        val price: PriceAmount,
        val distanceMeters: Double,
        val navigationQuery: String,
        val stationName: String,
    ) : FindNearestStationOutcome

    data object StationDetailMissing : FindNearestStationOutcome
    data object NoStations : FindNearestStationOutcome
    data object NoStationWithinRadius : FindNearestStationOutcome
    data object GeocodingFailed : FindNearestStationOutcome
}

/**
 * UC-015 — Finds the cheapest station close to the user for a fuel product and builds
 * the external navigation query (BR-028, BR-021, BR-026).
 *
 * Recommendations are restricted to the user-configured search radius (3/5/10/15 km,
 * default [NearestBestPriceStationRule.MAX_SEARCH_RADIUS_METERS]); when every geocoded
 * candidate lies beyond that radius the outcome is
 * [FindNearestStationOutcome.NoStationWithinRadius] instead of a navigation query.
 *
 * Candidate addresses are resolved in ascending price order, at most
 * [NearestBestPriceStationRule.MAX_GEOCODE_CANDIDATES] candidates. Each candidate is
 * geocoded through a fallback ladder — structured street search (with number, then
 * street-only) before the free-text query derived by [StationAddressNormalizationRule]
 * — because structured Nominatim queries resolve Brazilian addresses far more
 * reliably, especially s/n and rodovia addresses. A network failure aborts the search
 * immediately instead of waiting for every candidate to time out.
 */
class FindNearestBestPriceStationUseCase(
    private val getStationPricesUseCase: GetStationPricesUseCase,
    private val addressGeocodeRepository: AddressGeocodeRepository,
    private val buildStationNavigationQueryUseCase: BuildStationNavigationQueryUseCase,
    private val userPreferencesRepository: UserPreferencesRepository,
) {

    suspend operator fun invoke(
        fuelProduct: FuelProduct,
        deviceLocation: GeoCoordinates,
    ): FindNearestStationOutcome {
        val stations = when (val stationsOutcome = getStationPricesUseCase(fuelProduct = fuelProduct)) {
            is StationPricesOutcome.StationDetailMissing -> return FindNearestStationOutcome.StationDetailMissing
            is StationPricesOutcome.Success -> stationsOutcome.stations
        }
        if (stations.isEmpty()) {
            return FindNearestStationOutcome.NoStations
        }

        val preferences = userPreferencesRepository.getPreferences()
        val searchRadiusMeters = preferences.getNearestStationRadiusMeters()
        val geocodedCandidates = NearestBestPriceStationRule
            .selectCandidates(stations)
            .mapNotNull { stationPrice ->
                val municipality = stationPrice.station.municipality.ifBlank {
                    preferences.preferredMunicipality?.trim().orEmpty()
                }
                val parsed = StationAddressParsingRule.parse(stationPrice.station)
                val requests = buildList {
                    // 1) Structured search with housenumber + street.
                    if (parsed.number != null) {
                        add(
                            GeocodeRequest.Structured(
                                street = "${parsed.number} ${parsed.street}",
                                city = municipality,
                                state = stationPrice.station.state.displayName,
                            ),
                        )
                    }
                    // 2) Structured search with the street only (covers S/N and
                    //    highway addresses where the number misleads the search).
                    add(
                        GeocodeRequest.Structured(
                            street = parsed.street,
                            city = municipality,
                            state = stationPrice.station.state.displayName,
                        ),
                    )
                    // 3) Free-text fallback exactly as before this ladder existed.
                    add(
                        GeocodeRequest.FreeText(
                            StationAddressNormalizationRule.buildGeocodingQuery(
                                station = stationPrice.station,
                                preferredMunicipality = preferences.preferredMunicipality,
                                preferredState = preferences.preferredState,
                            ),
                        ),
                    )
                }
                when (val geocodeOutcome = addressGeocodeRepository.geocode(requests)) {
                    is AddressGeocodeOutcome.Success -> NearestBestPriceStationRule.Candidate(
                        station = stationPrice.station,
                        price = stationPrice.price,
                        coordinates = geocodeOutcome.coordinates,
                    )
                    AddressGeocodeOutcome.NotFound -> null
                    AddressGeocodeOutcome.NetworkError,
                    AddressGeocodeOutcome.RateLimited,
                    -> return FindNearestStationOutcome.GeocodingFailed
                }
            }

        if (geocodedCandidates.isEmpty()) {
            return FindNearestStationOutcome.GeocodingFailed
        }

        // Candidates exist, so a null selection means every candidate is beyond the radius.
        val recommendation = NearestBestPriceStationRule.select(
            candidates = geocodedCandidates,
            deviceLocation = deviceLocation,
            searchRadiusMeters = searchRadiusMeters,
        ) ?: return FindNearestStationOutcome.NoStationWithinRadius

        val navigation = buildStationNavigationQueryUseCase(recommendation.station)

        return FindNearestStationOutcome.Success(
            station = recommendation.station,
            price = recommendation.price,
            distanceMeters = recommendation.distanceMeters,
            navigationQuery = navigation.navigationQuery,
            stationName = recommendation.station.displayName().trim(),
        )
    }
}
