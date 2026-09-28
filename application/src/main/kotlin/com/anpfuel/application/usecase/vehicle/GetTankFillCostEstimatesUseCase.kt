package com.anpfuel.application.usecase.vehicle

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.TankFillCostEstimate
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.repository.AveragePriceRepository
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.StationPriceRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.ActiveSurveyWeekRule
import com.anpfuel.domain.rule.SearchRequiresImportedDataRule
import com.anpfuel.domain.rule.TankFillCostRule
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.domain.model.StationPrice

/**
 * UC-011 — Computes tank fill cost estimates for registered vehicles (BR-023, BR-024).
 */
data class VehicleTankFillCostEstimate(
    val vehicle: Vehicle,
    val estimate: TankFillCostEstimate?,
)

data class TankFillCostEstimatesResult(
    val surveyWeek: SurveyWeek,
    val state: BrazilianState,
    val municipality: String,
    val items: List<VehicleTankFillCostEstimate>,
)

class GetTankFillCostEstimatesUseCase(
    private val averagePriceRepository: AveragePriceRepository,
    private val stationPriceRepository: StationPriceRepository,
    private val priceTableRepository: PriceTableRepository,
    private val userPreferencesRepository: UserPreferencesRepository,
) {

    suspend operator fun invoke(
        vehicles: List<Vehicle>,
        state: BrazilianState? = null,
        municipality: String? = null,
        surveyWeek: SurveyWeek? = null,
    ): TankFillCostEstimatesResult {
        SearchRequiresImportedDataRule.validate(
            priceTableRepository.countImportedSurveyWeeks(),
        )

        val resolvedState = state ?: requirePreferredState()
        val resolvedMunicipality = municipality?.trim()?.takeIf { it.isNotBlank() }
            ?: requirePreferredMunicipality()
        val resolvedSurveyWeek = surveyWeek ?: resolveDisplaySurveyWeek()

        if (vehicles.isEmpty()) {
            return TankFillCostEstimatesResult(
                surveyWeek = resolvedSurveyWeek,
                state = resolvedState,
                municipality = resolvedMunicipality,
                items = emptyList(),
            )
        }

        val averagePrices = averagePriceRepository.getPricesByMunicipality(
            state = resolvedState,
            municipality = resolvedMunicipality,
            surveyWeek = resolvedSurveyWeek,
        ).associateBy { it.fuelProduct }

        val hasStationData = stationPriceRepository.hasStationData(
            surveyWeek = resolvedSurveyWeek,
            state = resolvedState,
            municipality = resolvedMunicipality,
        )

        val stationPricesByProduct = mutableMapOf<FuelProduct, List<StationPrice>>()
        val items = vehicles
            .sortedBy { it.sortOrder }
            .map { vehicle ->
                val stationPrices = if (hasStationData) {
                    stationPricesByProduct.getOrPut(vehicle.fuelProduct) {
                        stationPriceRepository.getStationPrices(
                            state = resolvedState,
                            municipality = resolvedMunicipality,
                            fuelProduct = vehicle.fuelProduct,
                            surveyWeek = resolvedSurveyWeek,
                        )
                    }
                } else {
                    emptyList()
                }

                val estimate = TankFillCostRule.estimate(
                    vehicle = vehicle,
                    context = TankFillCostRule.PriceContext(
                        stationPrices = stationPrices,
                        averagePrice = averagePrices[vehicle.fuelProduct],
                    ),
                )

                VehicleTankFillCostEstimate(
                    vehicle = vehicle,
                    estimate = estimate,
                )
            }

        return TankFillCostEstimatesResult(
            surveyWeek = resolvedSurveyWeek,
            state = resolvedState,
            municipality = resolvedMunicipality,
            items = items,
        )
    }

    private suspend fun requirePreferredState(): BrazilianState =
        userPreferencesRepository.getPreferences().preferredState
            ?: throw DomainException("Preferred state is not set")

    private suspend fun requirePreferredMunicipality(): String =
        userPreferencesRepository.getPreferences().preferredMunicipality?.trim()?.takeIf { it.isNotBlank() }
            ?: throw DomainException("Preferred municipality is not set")

    private suspend fun resolveDisplaySurveyWeek(): SurveyWeek {
        val preferences = userPreferencesRepository.getPreferences()
        val importedSurveys = priceTableRepository.getImportedPriceSurveys()
        return ActiveSurveyWeekRule.resolveDisplayWeek(
            activeSurveyWeek = preferences.activeSurveyWeek,
            importedSurveys = importedSurveys,
        ) ?: throw DomainException("BR-006: No successfully imported SurveyWeek is available")
    }
}
