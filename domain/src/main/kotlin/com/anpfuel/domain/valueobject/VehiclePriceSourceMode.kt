package com.anpfuel.domain.valueobject

/**
 * How a [com.anpfuel.domain.model.Vehicle] resolves unit price (BR-023, BR-025).
 */
enum class VehiclePriceSourceMode {
    CHEAPEST_STATION,
    SPECIFIC_STATION,
}
