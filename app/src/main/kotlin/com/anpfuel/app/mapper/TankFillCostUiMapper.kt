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
        // BEV carries no tank and no fuel, so it never appears in the
        // fuel fill card (energy comparisons belong to EE-23).
        items.mapNotNull { item -> toUiModel(item, locale, state, municipality) }

    fun toUiModel(
        item: VehicleTankFillCostEstimate,
        locale: Locale,
        state: BrazilianState,
        municipality: String,
    ): TankFillCostEstimateUiModel? {
        val tankCapacity = item.vehicle.tankCapacity ?: return null
        val fuelProduct = item.vehicle.fuelProduct ?: return null
        val estimate = item.estimate
        val referenceStation = estimate?.referenceStation
        return TankFillCostEstimateUiModel(
            vehicleId = item.vehicle.id,
            displayName = item.vehicle.displayName,
            tankCapacityLiters = tankCapacity.liters.toInt(),
            tankCapacityLitersLabel = tankCapacity.liters
                .stripTrailingZeros()
                .toPlainString(),
            fuelProduct = fuelProduct,
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
            photoPath = item.vehicle.photoPath,
        )
    }
}
