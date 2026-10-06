package com.anpfuel.app.navigation

import com.anpfuel.domain.valueobject.FuelProduct

object Routes {
    const val ONBOARDING = "onboarding"
    const val AUTH = "auth"
    const val HOME = "home"
    const val EXPLORE = "home"
    const val COMMUNITY = "community"
    const val PROFILE = "profile"
    const val SEARCH = "search"
    const val LOCATION = "location"
    const val PRICES = "prices"
    const val HISTORY = "history"
    const val STATIONS = "stations"
    const val STATIONS_WITH_FUEL = "stations/{fuelProduct}"
    const val SETTINGS = "settings"
    const val HELP = "help"
    const val VEHICLES = "vehicles"
    const val WEEK_PICKER = "week_picker"
    const val CAPTURE = "capture"
    const val CAPTURE_WITH_TARGET = "capture?stationId={stationId}&fuel={fuel}"
    const val STATION_CLAIM = "station-claim/{stationId}"
    const val STATION_MANAGEMENT = "station-management/{stationId}"
    fun stationClaim(stationId: String): String = "station-claim/$stationId"
    fun stationManagement(stationId: String): String = "station-management/$stationId"

    const val STATION_PAGE = "station/{stationKey}?fuelProduct={fuelProduct}"
    fun stationPage(stationKey: String, fuel: FuelProduct): String {
        require(com.anpfuel.domain.discovery.StationPageIdentity.parse(stationKey) != null) { "invalid station identity" }
        return "station/$stationKey?fuelProduct=${fuel.name}"
    }

    const val STATION_PROFILE = "station-profile/{stationId}"

    fun stationProfile(stationId: String): String = "station-profile/$stationId"

    const val SUGGEST = "suggest"

    fun stations(fuelProduct: FuelProduct): String = "stations/${fuelProduct.name}"

    /** Contextual capture for one canonical station + wire fuel (P37-T01). */
    fun capture(stationId: String, fuelProductWire: String): String =
        "capture?stationId=$stationId&fuel=$fuelProductWire"
}
