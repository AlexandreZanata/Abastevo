package com.anpfuel.application.usecase.station

import com.anpfuel.domain.event.StationNavigationRequested
import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.StationAddressNormalizationRule

data class BuildStationNavigationQueryResult(
    val navigationQuery: String,
    val event: StationNavigationRequested,
)

/**
 * UC-013 — Builds normalized geo search query and records navigation intent.
 */
class BuildStationNavigationQueryUseCase(
    private val userPreferencesRepository: UserPreferencesRepository,
    private val eventPublisher: DomainEventPublisher,
) {

    suspend fun buildQuery(station: RetailStation): String {
        val preferences = userPreferencesRepository.getPreferences()
        return StationAddressNormalizationRule.buildNavigationQuery(
            station = station,
            preferredMunicipality = preferences.preferredMunicipality,
            preferredState = preferences.preferredState,
        )
    }

    suspend operator fun invoke(station: RetailStation): BuildStationNavigationQueryResult {
        val navigationQuery = buildQuery(station)
        val event = StationNavigationRequested.create(
            payload = StationNavigationRequested.Payload(
                cnpj = station.cnpj,
                navigationQuery = navigationQuery,
            ),
        )
        eventPublisher.publish(event)
        return BuildStationNavigationQueryResult(
            navigationQuery = navigationQuery,
            event = event,
        )
    }
}
