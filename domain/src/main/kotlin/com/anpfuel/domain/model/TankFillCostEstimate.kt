package com.anpfuel.domain.model

import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.TankCapacity

/**
 * Resolved tank fill cost for a [Vehicle] and active survey week (UC-011).
 */
data class TankFillCostEstimate(
    val vehicleId: DomainId,
    val displayName: String,
    val tankCapacity: TankCapacity,
    val fuelProduct: FuelProduct,
    val unitPrice: PriceAmount,
    val totalCost: PriceAmount,
    val unitPriceSource: TankFillCostUnitPriceSource,
    val stationDisplayName: String? = null,
    val referenceStation: RetailStation? = null,
)

enum class TankFillCostUnitPriceSource {
    CHEAPEST_STATION,
    SPECIFIC_STATION,
    AVERAGE_MINIMUM,
}
