package com.anpfuel.application.port

/**
 * MEX-12 per-vehicle estimated efficiency configuration (km/L text as
 * entered, validated at the use case by [EstimatedEfficiency]).
 * Local-only; never uploaded, never a measurement claim.
 */
interface EstimatedEfficiencyStore {
    fun get(vehicleId: String): String?
    fun set(vehicleId: String, kmPerLitre: String)
    fun clear(vehicleId: String)
}
