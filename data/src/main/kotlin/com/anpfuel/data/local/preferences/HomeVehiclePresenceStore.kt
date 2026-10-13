package com.anpfuel.data.local.preferences

import android.content.Context
import android.content.SharedPreferences
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Remembers whether the driver has a registered vehicle.
 *
 * Synchronous SharedPreferences flag so Home can pick its loading
 * skeleton (vehicle carousel vs contribution hero) before the first
 * frame, without flashing the hero for drivers. Refreshed on every
 * successful home load; never gates content, only the placeholder.
 */
@Singleton
class HomeVehiclePresenceStore @Inject constructor(
    @ApplicationContext context: Context,
) {

    private val prefs: SharedPreferences =
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    fun hasVehicle(): Boolean =
        prefs.getBoolean(KEY_HAS_VEHICLE, false)

    fun setHasVehicle(hasVehicle: Boolean) {
        prefs.edit().putBoolean(KEY_HAS_VEHICLE, hasVehicle).apply()
    }

    companion object {
        const val PREFS_NAME = "home"
        const val KEY_HAS_VEHICLE = "home_has_vehicle"
    }
}
