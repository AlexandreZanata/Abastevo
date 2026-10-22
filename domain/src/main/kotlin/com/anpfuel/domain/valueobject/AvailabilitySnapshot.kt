package com.anpfuel.domain.valueobject

/**
 * MEX-04 — Batch availability resolution for one survey week (or none).
 * Both sets hold canonical catalog keys; an empty [withDataThisWeek] with a
 * null week is not an error, it means no week is available for enrichment.
 */
data class AvailabilitySnapshot(
    val withDataThisWeek: Set<MunicipalityLocationKey>,
    val everInAnp: Set<MunicipalityLocationKey>,
) {
    companion object {
        fun empty(): AvailabilitySnapshot =
            AvailabilitySnapshot(
                withDataThisWeek = emptySet(),
                everInAnp = emptySet(),
            )
    }
}
