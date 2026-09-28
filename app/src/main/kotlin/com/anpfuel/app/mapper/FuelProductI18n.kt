package com.anpfuel.app.mapper

import androidx.annotation.StringRes
import com.anpfuel.app.R
import com.anpfuel.domain.valueobject.FuelProduct

object FuelProductI18n {

    @StringRes
    fun toStringRes(product: FuelProduct): Int = when (product) {
        FuelProduct.ETHANOL -> R.string.fuel_product_ethanol
        FuelProduct.GASOLINE_REGULAR -> R.string.fuel_product_gasoline_regular
        FuelProduct.GASOLINE_PREMIUM -> R.string.fuel_product_gasoline_premium
        FuelProduct.DIESEL_S500 -> R.string.fuel_product_diesel_s500
        FuelProduct.DIESEL_S10 -> R.string.fuel_product_diesel_s10
        FuelProduct.CNG -> R.string.fuel_product_cng
        FuelProduct.LPG_P13 -> R.string.fuel_product_lpg_p13
    }
}
