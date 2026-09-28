package com.anpfuel.application.usecase.alert

import com.anpfuel.application.format.BrlPriceFormatter
import com.anpfuel.domain.model.PriceDropAlertNotification
import com.anpfuel.domain.repository.AveragePriceRepository
import com.anpfuel.domain.repository.PriceDropNotificationRepository
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.StationPriceRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.repository.VehicleRepository
import com.anpfuel.domain.rule.ActiveSurveyWeekRule
import com.anpfuel.domain.rule.PriceDropDetectionRule
import com.anpfuel.domain.rule.PreviousSurveyWeekRule
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek

enum class PriceDropAlertSkipReason {
    PERMISSION_DENIED,
    NO_ALERTS_ENABLED,
    NO_LOCATION,
    NO_CURRENT_WEEK,
    NO_PREVIOUS_WEEK,
}

data class EvaluatePriceDropAlertsResult(
    val notificationsShown: Int,
    val skipReason: PriceDropAlertSkipReason? = null,
)

/**
 * UC-014 — Evaluates configured vehicles after a successful weekly import.
 */
class EvaluatePriceDropAlertsUseCase(
    private val vehicleRepository: VehicleRepository,
    private val averagePriceRepository: AveragePriceRepository,
    private val stationPriceRepository: StationPriceRepository,
    private val priceTableRepository: PriceTableRepository,
    private val userPreferencesRepository: UserPreferencesRepository,
    private val priceDropNotificationRepository: PriceDropNotificationRepository,
) {

    suspend operator fun invoke(): EvaluatePriceDropAlertsResult {
        if (!priceDropNotificationRepository.hasPostNotificationsPermission()) {
            return EvaluatePriceDropAlertsResult(
                notificationsShown = 0,
                skipReason = PriceDropAlertSkipReason.PERMISSION_DENIED,
            )
        }

        val alertVehicles = vehicleRepository.listAll().filter { it.priceDropAlertEnabled }
        if (alertVehicles.isEmpty()) {
            return EvaluatePriceDropAlertsResult(
                notificationsShown = 0,
                skipReason = PriceDropAlertSkipReason.NO_ALERTS_ENABLED,
            )
        }

        val preferences = userPreferencesRepository.getPreferences()
        val state = preferences.preferredState
            ?: return EvaluatePriceDropAlertsResult(0, PriceDropAlertSkipReason.NO_LOCATION)
        val municipality = preferences.preferredMunicipality?.trim()?.takeIf { it.isNotBlank() }
            ?: return EvaluatePriceDropAlertsResult(0, PriceDropAlertSkipReason.NO_LOCATION)

        val importedSurveys = priceTableRepository.getImportedPriceSurveys()
        val currentWeek = ActiveSurveyWeekRule.resolveDisplayWeek(
            activeSurveyWeek = preferences.activeSurveyWeek,
            importedSurveys = importedSurveys,
        ) ?: return EvaluatePriceDropAlertsResult(0, PriceDropAlertSkipReason.NO_CURRENT_WEEK)

        val previousWeek = PreviousSurveyWeekRule.resolvePreviousWeek(
            currentWeek = currentWeek,
            importedSurveys = importedSurveys,
        ) ?: return EvaluatePriceDropAlertsResult(0, PriceDropAlertSkipReason.NO_PREVIOUS_WEEK)

        val stationCache = mutableMapOf<WeekFuelKey, List<com.anpfuel.domain.model.StationPrice>>()
        var notificationsShown = 0
        for (vehicle in alertVehicles) {
            val currentContext = buildWeekContextForVehicle(
                state = state,
                municipality = municipality,
                surveyWeek = currentWeek,
                fuelProduct = vehicle.fuelProduct,
                stationCache = stationCache,
            )
            val previousContext = buildWeekContextForVehicle(
                state = state,
                municipality = municipality,
                surveyWeek = previousWeek,
                fuelProduct = vehicle.fuelProduct,
                stationCache = stationCache,
            )
            val (currentPrice, previousPrice) = PriceDropDetectionRule.resolveComparisonPrices(
                vehicle = vehicle,
                currentWeek = currentContext,
                previousWeek = previousContext,
            )
            if (!PriceDropDetectionRule.shouldNotify(currentPrice, previousPrice)) {
                continue
            }

            priceDropNotificationRepository.showPriceDropAlert(
                PriceDropAlertNotification(
                    vehicleId = vehicle.id,
                    vehicleDisplayName = vehicle.displayName,
                    currentPriceFormatted = BrlPriceFormatter.format(currentPrice!!),
                ),
            )
            notificationsShown++
        }

        return EvaluatePriceDropAlertsResult(notificationsShown = notificationsShown)
    }

    private suspend fun buildWeekContextForVehicle(
        state: BrazilianState,
        municipality: String,
        surveyWeek: SurveyWeek,
        fuelProduct: FuelProduct,
        stationCache: MutableMap<WeekFuelKey, List<com.anpfuel.domain.model.StationPrice>>,
    ): PriceDropDetectionRule.WeekPriceContext {
        val cacheKey = WeekFuelKey(surveyWeek, fuelProduct)
        val stationPrices = stationCache.getOrPut(cacheKey) {
            if (stationPriceRepository.hasStationData(
                    surveyWeek = surveyWeek,
                    state = state,
                    municipality = municipality,
                )
            ) {
                stationPriceRepository.getStationPrices(
                    state = state,
                    municipality = municipality,
                    fuelProduct = fuelProduct,
                    surveyWeek = surveyWeek,
                )
            } else {
                emptyList()
            }
        }
        val averagePrice = averagePriceRepository.getPricesByMunicipality(
            state = state,
            municipality = municipality,
            surveyWeek = surveyWeek,
        ).firstOrNull { it.fuelProduct == fuelProduct }

        return PriceDropDetectionRule.WeekPriceContext(
            stationPrices = stationPrices,
            averagePrice = averagePrice,
        )
    }

    private data class WeekFuelKey(
        val surveyWeek: SurveyWeek,
        val fuelProduct: FuelProduct,
    )
}
