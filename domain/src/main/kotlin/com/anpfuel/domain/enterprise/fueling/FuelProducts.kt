package com.anpfuel.domain.enterprise.fueling

import com.anpfuel.domain.enterprise.fleet.CatalogProduct

enum class FuelFailure { INVALID, SCOPE, PRODUCT_UNIT_MISMATCH }
class FuelDomainException(val failure: FuelFailure = FuelFailure.INVALID) : IllegalArgumentException("fuel: ${failure.name.lowercase()}")
internal fun invalid(): FuelDomainException = FuelDomainException()
internal fun scope(): FuelDomainException = FuelDomainException(FuelFailure.SCOPE)
enum class FuelCategory { LIQUID_FUEL, GAS_FUEL, ELECTRICITY, ADDITIVE }
enum class FuelUnit { L, M3, KWH }
enum class StorageUnit { ML, MILLI_M3, WH }
enum class FuelFamily { ETHANOL, GASOLINE, DIESEL, NATURAL_GAS, ELECTRIC, NONE }
enum class FuelProduct(val category: FuelCategory, val unit: FuelUnit, val storage: StorageUnit, val family: FuelFamily) {
    ETHANOL(FuelCategory.LIQUID_FUEL, FuelUnit.L, StorageUnit.ML, FuelFamily.ETHANOL),
    GASOLINE_REGULAR(FuelCategory.LIQUID_FUEL, FuelUnit.L, StorageUnit.ML, FuelFamily.GASOLINE),
    GASOLINE_ADDITIVED(FuelCategory.LIQUID_FUEL, FuelUnit.L, StorageUnit.ML, FuelFamily.GASOLINE),
    GASOLINE_PREMIUM_GRADE(FuelCategory.LIQUID_FUEL, FuelUnit.L, StorageUnit.ML, FuelFamily.GASOLINE),
    DIESEL_S10(FuelCategory.LIQUID_FUEL, FuelUnit.L, StorageUnit.ML, FuelFamily.DIESEL),
    DIESEL_S500(FuelCategory.LIQUID_FUEL, FuelUnit.L, StorageUnit.ML, FuelFamily.DIESEL),
    CNG(FuelCategory.GAS_FUEL, FuelUnit.M3, StorageUnit.MILLI_M3, FuelFamily.NATURAL_GAS),
    ELECTRICITY(FuelCategory.ELECTRICITY, FuelUnit.KWH, StorageUnit.WH, FuelFamily.ELECTRIC),
    ARLA_32(FuelCategory.ADDITIVE, FuelUnit.L, StorageUnit.ML, FuelFamily.NONE);
    val isAdditive: Boolean get() = category == FuelCategory.ADDITIVE
}

object FuelProducts {
    const val VERSION = "fuel-products-v1"
    fun get(code: String): FuelProduct = FuelProduct.entries.firstOrNull { it.name == code } ?: throw invalid()
    fun all(): List<FuelProduct> = FuelProduct.entries.toList()
    fun fleetProduct(code: String): CatalogProduct? = FuelProduct.entries.firstOrNull { it.name == code }?.let { CatalogProduct(it.name, it.unit.name) }
}
