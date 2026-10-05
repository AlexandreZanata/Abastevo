package com.anpfuel.domain.profile

/**
 * P32-T01 — Public station profile projection (no private data).
 *
 * Mirrors backend `stationprofile` P30 contract (`profile-v1`): only
 * [PUBLIC_BUSINESS_KEYS] survive the public projection. CPF, keys,
 * passwords, precise GPS and prices are never public here.
 */
val PUBLIC_BUSINESS_KEYS: Set<String> = setOf(
    "opening_hours",
    "services",
    "phone",
    "website",
    "description",
)

data class StationProfile(
    val stationId: String,
    val displayName: String,
    val business: Map<String, String> = emptyMap(),
    val revision: Int = 0,
    val operatorSource: String = "",
    val hasBadge: Boolean = false,
) {
    companion object {
        fun projectBusiness(raw: Map<String, String>): Map<String, String> =
            raw.filterKeys { PUBLIC_BUSINESS_KEYS.contains(it) }
    }
}
