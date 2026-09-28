package com.anpfuel.app.navigation

import com.anpfuel.domain.valueobject.FuelProduct

object Routes {
    const val ONBOARDING = "onboarding"
    const val HOME = "home"
    const val SEARCH = "search"
    const val LOCATION = "location"
    const val PRICES = "prices"
    const val HISTORY = "history"
    const val STATIONS = "stations"
    const val STATIONS_WITH_FUEL = "stations/{fuelProduct}"
    const val SETTINGS = "settings"
    const val VEHICLES = "vehicles"
    const val WEEK_PICKER = "week_picker"

    fun stations(fuelProduct: FuelProduct): String = "stations/${fuelProduct.name}"
}
