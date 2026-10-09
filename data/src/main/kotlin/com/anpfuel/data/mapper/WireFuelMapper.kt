package com.anpfuel.data.mapper

import com.anpfuel.domain.valueobject.FuelProduct

/**
 * P10-T01 wire vocabulary bridge (A05).
 *
 * The backend wire enum uses GASOLINE_ADDITIVED; the legacy Android enum
 * keeps GASOLINE_PREMIUM. This adapter owns the bridge in one place:
 * [toWire] maps the legacy premium to the wire name, [fromWire] accepts
 * the supported wire values and refuses the legacy name plus unknown
 * enums without coercion. No package moves, no enum renames.
 */
object WireFuelMapper {

    private val toWireMap: Map<FuelProduct, String> = mapOf(
        FuelProduct.ETHANOL to "ETHANOL",
        FuelProduct.GASOLINE_REGULAR to "GASOLINE_REGULAR",
        FuelProduct.GASOLINE_PREMIUM to "GASOLINE_ADDITIVED",
        FuelProduct.DIESEL_S500 to "DIESEL_S500",
        FuelProduct.DIESEL_S10 to "DIESEL_S10",
        FuelProduct.CNG to "CNG",
        FuelProduct.LPG_P13 to "LPG_P13",
        FuelProduct.GASOLINE_PREMIUM_GRADE to "GASOLINE_PREMIUM_GRADE",
    )

    private val fromWireMap: Map<String, FuelProduct> =
        toWireMap.entries.associate { (product, wire) -> wire to product }

    fun wireValues(): Set<String> = fromWireMap.keys

    fun toWire(product: FuelProduct): String =
        requireNotNull(toWireMap[product]) { "unmapped fuel product: $product" }

    fun fromWire(wire: String): Result<FuelProduct> {
        val product = fromWireMap[wire]
        return if (product != null) {
            Result.success(product)
        } else {
            Result.failure(IllegalArgumentException("unknown wire fuel product: $wire"))
        }
    }
}
