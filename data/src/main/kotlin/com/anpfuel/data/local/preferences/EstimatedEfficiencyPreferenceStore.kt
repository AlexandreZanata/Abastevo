package com.anpfuel.data.local.preferences

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.application.port.EstimatedEfficiencyStore
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * MEX-12 local-only estimated km/L per vehicle. Plain SharedPreferences
 * (synchronous, main-safe for tiny reads) keyed by vehicle ID; validation
 * stays in the use case so corrupt rows fail closed instead of coercing.
 */
@Singleton
class EstimatedEfficiencyPreferenceStore internal constructor(
    private val read: (String) -> String?,
    private val write: (String, String?) -> Unit,
) : EstimatedEfficiencyStore {

    @Inject
    constructor(@ApplicationContext context: Context) : this(
        read = { key ->
            context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE).getString(key, null)
        },
        write = { key, value ->
            val prefs: SharedPreferences =
                context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
            if (value == null) prefs.edit().remove(key).apply()
            else prefs.edit().putString(key, value).apply()
        },
    )

    override fun get(vehicleId: String): String? = read(keyFor(vehicleId))

    override fun set(vehicleId: String, kmPerLitre: String) {
        write(keyFor(vehicleId), kmPerLitre)
    }

    override fun clear(vehicleId: String) {
        write(keyFor(vehicleId), null)
    }

    companion object {
        const val PREFS_NAME = "estimated_efficiency"
        fun keyFor(vehicleId: String): String = "km_per_l|$vehicleId"
    }
}
