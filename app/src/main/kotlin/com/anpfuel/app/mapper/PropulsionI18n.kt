package com.anpfuel.app.mapper

import androidx.annotation.StringRes
import com.anpfuel.app.R
import com.anpfuel.domain.valueobject.Propulsion

object PropulsionI18n {

    @StringRes
    fun toStringRes(propulsion: Propulsion): Int = when (propulsion) {
        Propulsion.ICE -> R.string.vehicle_propulsion_ice
        Propulsion.BEV -> R.string.vehicle_propulsion_bev
        Propulsion.PHEV -> R.string.vehicle_propulsion_phev
        Propulsion.HEV -> R.string.vehicle_propulsion_hev
        Propulsion.MHEV -> R.string.vehicle_propulsion_mhev
    }
}
