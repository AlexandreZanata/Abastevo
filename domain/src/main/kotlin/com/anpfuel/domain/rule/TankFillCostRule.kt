package com.anpfuel.domain.rule

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.model.TankFillCostEstimate
import com.anpfuel.domain.model.TankFillCostUnitPriceSource
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSourceMode
import java.math.BigDecimal
import java.math.RoundingMode

/**
 * BR-023 — Resolve unit price and total tank fill cost for a vehicle.
 */
object TankFillCostRule {

    data class PriceContext(
        val stationPrices: List<StationPrice>,
        val averagePrice: AveragePrice?,
    )

    fun estimate(vehicle: Vehicle, context: PriceContext): TankFillCostEstimate? {
        val resolved = resolveUnitPrice(vehicle, context) ?: return null
        val totalCost = multiply(resolved.unitPrice, vehicle.tankCapacity)
        return TankFillCostEstimate(
            vehicleId = vehicle.id,
            displayName = vehicle.displayName,
            tankCapacity = vehicle.tankCapacity,
            fuelProduct = vehicle.fuelProduct,
            unitPrice = resolved.unitPrice,
            totalCost = totalCost,
            unitPriceSource = resolved.source,
            stationDisplayName = resolved.stationDisplayName,
            referenceStation = resolved.referenceStation,
        )
    }

    fun resolveUnitPrice(vehicle: Vehicle, context: PriceContext): ResolvedUnitPrice? {
        VehicleFuelProductRule.requireVehicleFuelProduct(vehicle)
        val matchingStations = context.stationPrices.filter { it.isForProduct(vehicle.fuelProduct) }

        return when (vehicle.priceSource.mode) {
            VehiclePriceSourceMode.CHEAPEST_STATION -> {
                cheapestStationPrice(matchingStations)
                    ?: averageMinimum(context.averagePrice)
            }
            VehiclePriceSourceMode.SPECIFIC_STATION -> {
                val cnpj = vehicle.priceSource.specificStationCnpj
                    ?: throw DomainException("SPECIFIC_STATION requires a CNPJ")
                specificStationPrice(matchingStations, cnpj)
                    ?: cheapestStationPrice(matchingStations)
                    ?: averageMinimum(context.averagePrice)
            }
        }
    }

    fun multiply(unitPrice: PriceAmount, capacity: TankCapacity): PriceAmount {
        val total = unitPrice.value
            .multiply(capacity.liters)
            .setScale(2, RoundingMode.HALF_UP)
        return PriceAmount.of(total)
    }

    private fun cheapestStationPrice(stations: List<StationPrice>): ResolvedUnitPrice? {
        val cheapest = stations.minByOrNull { it.price.value } ?: return null
        return ResolvedUnitPrice(
            unitPrice = cheapest.price,
            source = TankFillCostUnitPriceSource.CHEAPEST_STATION,
            stationDisplayName = cheapest.station.displayName(),
            referenceStation = cheapest.station,
        )
    }

    private fun specificStationPrice(
        stations: List<StationPrice>,
        cnpj: com.anpfuel.domain.valueobject.Cnpj,
    ): ResolvedUnitPrice? {
        val match = stations.firstOrNull { it.station.cnpj == cnpj } ?: return null
        return ResolvedUnitPrice(
            unitPrice = match.price,
            source = TankFillCostUnitPriceSource.SPECIFIC_STATION,
            stationDisplayName = match.station.displayName(),
            referenceStation = match.station,
        )
    }

    private fun averageMinimum(averagePrice: AveragePrice?): ResolvedUnitPrice? {
        val minimum = averagePrice?.minimum ?: return null
        return ResolvedUnitPrice(
            unitPrice = minimum,
            source = TankFillCostUnitPriceSource.AVERAGE_MINIMUM,
            stationDisplayName = null,
        )
    }

    data class ResolvedUnitPrice(
        val unitPrice: PriceAmount,
        val source: TankFillCostUnitPriceSource,
        val stationDisplayName: String?,
        val referenceStation: RetailStation? = null,
    )
}
