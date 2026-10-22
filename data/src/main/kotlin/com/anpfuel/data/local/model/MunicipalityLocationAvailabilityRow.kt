package com.anpfuel.data.local.model

/**
 * MEX-04 — One grouped availability row per imported (state, municipality).
 * [hasWeekData] is 1 when the location has rows for the requested survey week,
 * 0 otherwise; the week filter never changes the returned location set.
 */
data class MunicipalityLocationAvailabilityRow(
    val state: String,
    val municipality: String,
    val hasWeekData: Boolean,
)
