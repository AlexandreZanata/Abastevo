package com.anpfuel.data.mapper

import com.anpfuel.domain.rule.FuelProductNormalizationRule
import com.anpfuel.domain.valueobject.FuelProduct

/**
 * Maps raw ANP Portuguese product labels to [FuelProduct] (BR-002).
 */
object AnpProductMapper {

    fun toFuelProduct(rawLabel: String): Result<FuelProduct> =
        FuelProductNormalizationRule.normalize(rawLabel)

    fun toFuelProductOrThrow(rawLabel: String): FuelProduct =
        toFuelProduct(rawLabel).getOrThrow()
}
