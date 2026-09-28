package com.anpfuel.domain.rule

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.VehiclePriceSourceMode

/**
 * BR-025 — Detect fuel price drops between survey weeks for alerts.
 */
object PriceDropDetectionRule {

    data class WeekPriceContext(
        val stationPrices: List<StationPrice>,
        val averagePrice: AveragePrice?,
    )

    fun shouldNotify(currentPrice: PriceAmount?, previousPrice: PriceAmount?): Boolean {
        if (currentPrice == null || previousPrice == null) {
            return false
        }
        return currentPrice.value.compareTo(previousPrice.value) < 0
    }

    fun resolveComparisonPrices(
        vehicle: Vehicle,
        currentWeek: WeekPriceContext,
        previousWeek: WeekPriceContext,
    ): Pair<PriceAmount?, PriceAmount?> {
        VehicleFuelProductRule.requireVehicleFuelProduct(vehicle)
        val current = resolvePrice(vehicle, currentWeek)
        val previous = resolvePrice(vehicle, previousWeek)
        return current to previous
    }

    fun resolvePrice(vehicle: Vehicle, context: WeekPriceContext): PriceAmount? {
        val matchingStations = context.stationPrices.filter { it.isForProduct(vehicle.fuelProduct) }
        return when (vehicle.priceSource.mode) {
            VehiclePriceSourceMode.CHEAPEST_STATION -> {
                cheapestStationPrice(matchingStations)?.price
                    ?: context.averagePrice?.minimum
            }
            VehiclePriceSourceMode.SPECIFIC_STATION -> {
                val cnpj = vehicle.priceSource.specificStationCnpj ?: return null
                specificStationPrice(matchingStations, cnpj)?.price
                    ?: cheapestStationPrice(matchingStations)?.price
                    ?: context.averagePrice?.minimum
            }
        }
    }

    private fun cheapestStationPrice(stations: List<StationPrice>): StationPrice? =
        stations.minByOrNull { it.price.value }

    private fun specificStationPrice(stations: List<StationPrice>, cnpj: Cnpj): StationPrice? =
        stations.firstOrNull { it.station.cnpj == cnpj }
}
