package com.anpfuel.data.remote.profile

import com.anpfuel.domain.profile.StationProfile

/**
 * P32-T01 — Server profile DTO (public projection only).
 *
 * Blank ids decode to null; business entries outside the frozen public
 * key set are dropped on [toDomain].
 */
data class StationProfileDto(
    val stationId: String,
    val displayName: String,
    val business: Map<String, String> = emptyMap(),
) {
    fun toDomain(): StationProfile? {
        if (stationId.isBlank()) return null
        return StationProfile(
            stationId = stationId,
            displayName = displayName,
            business = StationProfile.projectBusiness(business),
        )
    }
}
