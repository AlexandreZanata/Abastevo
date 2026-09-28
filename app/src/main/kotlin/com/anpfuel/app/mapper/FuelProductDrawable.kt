package com.anpfuel.app.mapper

import androidx.annotation.DrawableRes
import com.anpfuel.app.R
import com.anpfuel.domain.valueobject.FuelProduct

object FuelProductDrawable {

    @DrawableRes
    fun toDrawableRes(product: FuelProduct): Int = when (product) {
        FuelProduct.ETHANOL -> R.drawable.ic_fuel_ethanol
        FuelProduct.GASOLINE_REGULAR -> R.drawable.ic_fuel_gasoline_regular
        FuelProduct.GASOLINE_PREMIUM -> R.drawable.ic_fuel_gasoline_premium
        FuelProduct.DIESEL_S500 -> R.drawable.ic_fuel_diesel_s500
        FuelProduct.DIESEL_S10 -> R.drawable.ic_fuel_diesel_s10
        FuelProduct.CNG -> R.drawable.ic_fuel_cng
        FuelProduct.LPG_P13 -> R.drawable.ic_fuel_lpg_p13
    }
}
