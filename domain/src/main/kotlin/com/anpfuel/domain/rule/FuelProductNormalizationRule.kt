package com.anpfuel.domain.rule

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.valueobject.FuelProduct

/**
 * BR-002 — Maps raw ANP Portuguese product labels to [FuelProduct].
 */
object FuelProductNormalizationRule {

    private val labelToProduct: Map<String, FuelProduct> = buildMap {
        put("ETANOL HIDRATADO", FuelProduct.ETHANOL)
        put("ETANOL", FuelProduct.ETHANOL)
        put("GASOLINA COMUM", FuelProduct.GASOLINE_REGULAR)
        put("GASOLINA ADITIVADA", FuelProduct.GASOLINE_PREMIUM)
        put("OLEO DIESEL", FuelProduct.DIESEL_S500)
        put("DIESEL S500", FuelProduct.DIESEL_S500)
        put("OLEO DIESEL S10", FuelProduct.DIESEL_S10)
        put("DIESEL S10", FuelProduct.DIESEL_S10)
        put("GNV", FuelProduct.CNG)
        put("GLP", FuelProduct.LPG_P13)
    }

    fun normalize(rawLabel: String): Result<FuelProduct> {
        val normalized = rawLabel.trim().uppercase()
        if (normalized.isEmpty()) {
            return Result.failure(DomainException("ANP fuel product label must not be blank"))
        }

        val product = labelToProduct[normalized]
            ?: return Result.failure(DomainException("Unknown ANP fuel product label: $rawLabel"))

        return Result.success(product)
    }

    fun normalizeOrNull(rawLabel: String): FuelProduct? = normalize(rawLabel).getOrNull()
}
