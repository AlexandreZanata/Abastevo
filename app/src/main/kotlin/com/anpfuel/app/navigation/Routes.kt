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

    fun stations(fuelProduct: FuelProduct): String = "stations/${fuelProduct.name}"

    /** Contextual capture for one canonical station + wire fuel (P37-T01). */
    fun capture(stationId: String, fuelProductWire: String): String =
        "capture?stationId=$stationId&fuel=$fuelProductWire"
}
