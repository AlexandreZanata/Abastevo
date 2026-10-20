package com.anpfuel.application.usecase.price

import com.anpfuel.application.error.AppError
import com.anpfuel.domain.discovery.DiscoveryQuery
import com.anpfuel.domain.discovery.DiscoverySort
import com.anpfuel.domain.discovery.DiscoveryStationsRule
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.StationPriceRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.ActiveSurveyWeekRule
import com.anpfuel.domain.rule.EmptyMunicipalityResultRule
import com.anpfuel.domain.rule.SearchRequiresImportedDataRule
import com.anpfuel.domain.rule.StationDetailOptInRule
import com.anpfuel.domain.rule.StationPriceOrderingRule
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.domain.exception.DomainException

/**
 * UC-007 — Loads station-level prices from local cache sorted by ascending price.
 */
sealed class StationPricesOutcome {
    data class Success(
        val surveyWeek: SurveyWeek,
        val state: BrazilianState,
        val municipality: String,
        val fuelProduct: FuelProduct,
        val stations: List<StationPrice>,
    ) : StationPricesOutcome() {
        val isEmpty: Boolean
            get() = EmptyMunicipalityResultRule.shouldReturnEmptyList(stations.size)
    }

    data class StationDetailMissing(
        val error: AppError = AppError.StationDetailNotSynced,
        val requiresOnDemandDownload: Boolean,
    ) : StationPricesOutcome()
}

/**
 * UC-007 — bounded discovery window over the same eligible set as the
 * full list. Filtering, ordering and pagination run in the local query;
 * the full-list read stays reserved for cheapest/nearest computation
 * over the complete eligible set.
 */
sealed class StationPricesPageOutcome {
    data class Success(
        val surveyWeek: SurveyWeek,
        val state: BrazilianState,
        val municipality: String,
        val fuelProduct: FuelProduct,
        val page: DiscoveryStationsRule.DiscoveryPage,
    ) : StationPricesPageOutcome() {
        val isEmpty: Boolean
            get() = EmptyMunicipalityResultRule.shouldReturnEmptyList(page.totalCount)
    }

    data class StationDetailMissing(
        val error: AppError = AppError.StationDetailNotSynced,
        val requiresOnDemandDownload: Boolean,
    ) : StationPricesPageOutcome()
}

class GetStationPricesUseCase(
    private val stationPriceRepository: StationPriceRepository,
    private val priceTableRepository: PriceTableRepository,
    private val userPreferencesRepository: UserPreferencesRepository,
) {

    suspend operator fun invoke(
        fuelProduct: FuelProduct? = null,
        state: BrazilianState? = null,
        municipality: String? = null,
        surveyWeek: SurveyWeek? = null,
    ): StationPricesOutcome {
        SearchRequiresImportedDataRule.validate(
            priceTableRepository.countImportedSurveyWeeks(),
        )

        val preferences = userPreferencesRepository.getPreferences()
        val resolvedFuelProduct = fuelProduct ?: preferences.preferredFuelProduct
            ?: throw DomainException("Fuel product is not set")
        val resolvedState = state ?: preferences.preferredState
            ?: throw DomainException("Preferred state is not set")
        val resolvedMunicipality = municipality?.trim()?.takeIf { it.isNotBlank() }
            ?: preferences.preferredMunicipality?.trim()?.takeIf { it.isNotBlank() }
            ?: throw DomainException("Preferred municipality is not set")
        val resolvedSurveyWeek = surveyWeek ?: resolveDisplaySurveyWeek()

        val hasLocalStationData = stationPriceRepository.hasStationData(
            surveyWeek = resolvedSurveyWeek,
            state = resolvedState,
            municipality = resolvedMunicipality,
        )
        if (!hasLocalStationData) {
            if (isStationDetailImported(resolvedSurveyWeek)) {
                return StationPricesOutcome.Success(
                    surveyWeek = resolvedSurveyWeek,
                    state = resolvedState,
                    municipality = resolvedMunicipality,
                    fuelProduct = resolvedFuelProduct,
                    stations = emptyList(),
                )
            }
            return StationPricesOutcome.StationDetailMissing(
                requiresOnDemandDownload = StationDetailOptInRule.requiresOnDemandDownload(
                    syncStationDetail = preferences.syncStationDetail,
                    hasLocalStationData = false,
                ),
            )
        }

        val stations = StationPriceOrderingRule.orderByPriceAscending(
            stationPriceRepository.getStationPrices(
                state = resolvedState,
                municipality = resolvedMunicipality,
                fuelProduct = resolvedFuelProduct,
                surveyWeek = resolvedSurveyWeek,
            ),
        )

        return StationPricesOutcome.Success(
            surveyWeek = resolvedSurveyWeek,
            state = resolvedState,
            municipality = resolvedMunicipality,
            fuelProduct = resolvedFuelProduct,
            stations = stations,
        )
    }

    suspend fun page(
        fuelProduct: FuelProduct? = null,
        state: BrazilianState? = null,
        municipality: String? = null,
        surveyWeek: SurveyWeek? = null,
        search: String? = null,
        sort: DiscoverySort = DiscoverySort.PRICE_ASC,
        page: Int = 0,
        pageSize: Int = DiscoveryQuery.DEFAULT_PAGE_SIZE,
    ): StationPricesPageOutcome {
        return when (val resolution = resolveData(fuelProduct, state, municipality, surveyWeek)) {
            is DataResolution.Missing -> if (resolution.empty) {
                StationPricesPageOutcome.Success(
                    surveyWeek = resolution.surveyWeek,
                    state = resolution.state,
                    municipality = resolution.municipality,
                    fuelProduct = resolution.fuelProduct,
                    page = DiscoveryStationsRule.DiscoveryPage(
                        items = emptyList(),
                        hasMore = false,
                        totalCount = 0,
                    ),
                )
            } else {
                StationPricesPageOutcome.StationDetailMissing(
                    requiresOnDemandDownload = resolution.requiresOnDemandDownload,
                )
            }
            is DataResolution.Ready -> StationPricesPageOutcome.Success(
                surveyWeek = resolution.context.surveyWeek,
                state = resolution.context.state,
                municipality = resolution.context.municipality,
                fuelProduct = resolution.context.fuelProduct,
                page = stationPriceRepository.getStationPricesPage(
                    state = resolution.context.state,
                    municipality = resolution.context.municipality,
                    fuelProduct = resolution.context.fuelProduct,
                    surveyWeek = resolution.context.surveyWeek,
                    search = search,
                    sort = sort,
                    page = page,
                    pageSize = pageSize,
                ),
            )
        }
    }

    suspend fun findStationPrice(
        state: BrazilianState,
        municipality: String,
        fuelProduct: FuelProduct,
        surveyWeek: SurveyWeek,
        cnpjDigits: String,
    ): StationPrice? = stationPriceRepository.findStationPrice(
        state = state,
        municipality = municipality,
        fuelProduct = fuelProduct,
        surveyWeek = surveyWeek,
        cnpjDigits = cnpjDigits,
    )

    private data class ResolvedContext(
        val surveyWeek: SurveyWeek,
        val state: BrazilianState,
        val municipality: String,
        val fuelProduct: FuelProduct,
    )

    private sealed interface DataResolution {
        data class Ready(val context: ResolvedContext) : DataResolution
        data class Missing(
            val surveyWeek: SurveyWeek,
            val state: BrazilianState,
            val municipality: String,
            val fuelProduct: FuelProduct,
            val empty: Boolean,
            val requiresOnDemandDownload: Boolean,
        ) : DataResolution
    }

    private suspend fun resolveData(
        fuelProduct: FuelProduct?,
        state: BrazilianState?,
        municipality: String?,
        surveyWeek: SurveyWeek?,
    ): DataResolution {
        SearchRequiresImportedDataRule.validate(
            priceTableRepository.countImportedSurveyWeeks(),
        )

        val preferences = userPreferencesRepository.getPreferences()
        val resolvedFuelProduct = fuelProduct ?: preferences.preferredFuelProduct
            ?: throw DomainException("Fuel product is not set")
        val resolvedState = state ?: preferences.preferredState
            ?: throw DomainException("Preferred state is not set")
        val resolvedMunicipality = municipality?.trim()?.takeIf { it.isNotBlank() }
            ?: preferences.preferredMunicipality?.trim()?.takeIf { it.isNotBlank() }
            ?: throw DomainException("Preferred municipality is not set")
        val resolvedSurveyWeek = surveyWeek ?: resolveDisplaySurveyWeek()

        val hasLocalStationData = stationPriceRepository.hasStationData(
            surveyWeek = resolvedSurveyWeek,
            state = resolvedState,
            municipality = resolvedMunicipality,
        )
        if (!hasLocalStationData) {
            if (isStationDetailImported(resolvedSurveyWeek)) {
                return DataResolution.Missing(
                    surveyWeek = resolvedSurveyWeek,
                    state = resolvedState,
                    municipality = resolvedMunicipality,
                    fuelProduct = resolvedFuelProduct,
                    empty = true,
                    requiresOnDemandDownload = false,
                )
            }
            return DataResolution.Missing(
                surveyWeek = resolvedSurveyWeek,
                state = resolvedState,
                municipality = resolvedMunicipality,
                fuelProduct = resolvedFuelProduct,
                empty = false,
                requiresOnDemandDownload = StationDetailOptInRule.requiresOnDemandDownload(
                    syncStationDetail = preferences.syncStationDetail,
                    hasLocalStationData = false,
                ),
            )
        }
        return DataResolution.Ready(
            ResolvedContext(resolvedSurveyWeek, resolvedState, resolvedMunicipality, resolvedFuelProduct),
        )
    }

    private suspend fun resolveDisplaySurveyWeek(): SurveyWeek {
        val preferences = userPreferencesRepository.getPreferences()
        val importedSurveys = priceTableRepository.getImportedPriceSurveys()
        return ActiveSurveyWeekRule.resolveDisplayWeek(
            activeSurveyWeek = preferences.activeSurveyWeek,
            importedSurveys = importedSurveys,
        ) ?: throw DomainException("BR-006: No successfully imported SurveyWeek is available")
    }

    private suspend fun isStationDetailImported(surveyWeek: SurveyWeek): Boolean =
        priceTableRepository.findPriceSurveyByWeek(surveyWeek)?.hasStationData == true
}
