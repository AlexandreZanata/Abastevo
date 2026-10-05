package com.anpfuel.application.usecase.profile

import com.anpfuel.domain.profile.StationProfile
import com.anpfuel.domain.repository.StationProfileGateway

/**
 * P32-T01 — Anonymous profile read.
 *
 * Blank ids are [StationProfileOutcome.Invalid] without touching the
 * gateway. Unknown profiles are [StationProfileOutcome.Unclaimed], never
 * a synthetic station.
 */
sealed interface StationProfileOutcome {
    data object Invalid : StationProfileOutcome

    data object Unclaimed : StationProfileOutcome

    data class Found(val profile: StationProfile) : StationProfileOutcome
}

class GetStationProfileUseCase(
    private val gateway: StationProfileGateway,
) {
    suspend operator fun invoke(stationId: String): StationProfileOutcome {
        if (stationId.isBlank()) return StationProfileOutcome.Invalid
        val profile = gateway.getProfile(stationId)
            ?: return StationProfileOutcome.Unclaimed
        return StationProfileOutcome.Found(profile)
    }
}
