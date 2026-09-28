package com.anpfuel.app.mapper

import com.anpfuel.app.ui.model.TankFillCostEstimateUiModel
import com.anpfuel.application.usecase.vehicle.VehicleTankFillCostEstimate
import com.anpfuel.domain.rule.StationAddressNormalizationRule
import com.anpfuel.domain.valueobject.BrazilianState
import java.util.Locale

object TankFillCostUiMapper {

    fun toUiModels(
        items: List<VehicleTankFillCostEstimate>,
        locale: Locale,
        state: BrazilianState,
        municipality: String,
    ): List<TankFillCostEstimateUiModel> =
        items.map { item -> toUiModel(item, locale, state, municipality) }

    fun toUiModel(
        item: VehicleTankFillCostEstimate,
        locale: Locale,
        state: BrazilianState,
        municipality: String,
    ): TankFillCostEstimateUiModel {
        val estimate = item.estimate
        val referenceStation = estimate?.referenceStation
        return TankFillCostEstimateUiModel(
            vehicleId = item.vehicle.id,
            displayName = item.vehicle.displayName,
            tankCapacityLiters = item.vehicle.tankCapacity.liters.toInt(),
            tankCapacityLitersLabel = item.vehicle.tankCapacity.liters
                .stripTrailingZeros()
                .toPlainString(),
            fuelProduct = item.vehicle.fuelProduct,
            totalCostFormatted = estimate?.totalCost?.let { PriceFormatter.formatAmount(it, locale) },
            stationDisplayName = estimate?.stationDisplayName,
            stationNavigationQuery = referenceStation?.let { station ->
                StationAddressNormalizationRule.buildNavigationQuery(
                    station = station,
                    preferredMunicipality = municipality,
                    preferredState = state,
                )
            },
            unitPriceSource = estimate?.unitPriceSource,
        )
    }
}
