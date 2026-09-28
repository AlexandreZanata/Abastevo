package com.anpfuel.app.ui.home

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.error.AppError
import com.anpfuel.application.error.AppErrorResolver
import com.anpfuel.application.usecase.location.SelectLocationUseCase
import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.application.usecase.price.GetMunicipalityPricesUseCase
import com.anpfuel.application.usecase.readiness.GetDataReadinessUseCase
import com.anpfuel.application.usecase.sync.SyncPriceTablesUseCase
import com.anpfuel.application.usecase.vehicle.GetTankFillCostEstimatesUseCase
import com.anpfuel.application.usecase.vehicle.ListVehiclesUseCase
import com.anpfuel.app.mapper.TankFillCostUiMapper
import com.anpfuel.app.ui.model.AveragePriceUiModel
import com.anpfuel.app.ui.model.TankFillCostEstimateUiModel
import com.anpfuel.domain.event.SyncRequestSource
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.state.DataReadinessState
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.SurveyWeek
import dagger.hilt.android.lifecycle.HiltViewModel
import java.util.Locale
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class HomeUiState(
    val isLoading: Boolean = true,
    val isRefreshing: Boolean = false,
    val readiness: DataReadinessState = DataReadinessState.EMPTY,
    val isOffline: Boolean = false,
    val municipality: String? = null,
    val state: BrazilianState? = null,
    val surveyWeek: SurveyWeek? = null,
    val prices: List<AveragePriceUiModel> = emptyList(),
    val tankFillCostEstimates: List<TankFillCostEstimateUiModel> = emptyList(),
    val hasLocation: Boolean = false,
    val hasCachedData: Boolean = false,
    val isEmptyMunicipality: Boolean = false,
    val dataAvailability: DataAvailability? = null,
    val operationalNote: String? = null,
    val error: AppError? = null,
    val errorMessage: String? = null,
)

@HiltViewModel
class HomeViewModel @Inject constructor(
    private val getDataReadinessUseCase: GetDataReadinessUseCase,
    private val getMunicipalityPricesUseCase: GetMunicipalityPricesUseCase,
    private val selectLocationUseCase: SelectLocationUseCase,
    private val syncPriceTablesUseCase: SyncPriceTablesUseCase,
    private val listVehiclesUseCase: ListVehiclesUseCase,
    private val getTankFillCostEstimatesUseCase: GetTankFillCostEstimatesUseCase,
    observeNetworkConnectivityUseCase: ObserveNetworkConnectivityUseCase,
) : ViewModel() {

    private val _uiState = MutableStateFlow(HomeUiState())
    val uiState: StateFlow<HomeUiState> = _uiState.asStateFlow()

    private var hasPresentedContent = false

    init {
        viewModelScope.launch {
            observeNetworkConnectivityUseCase().collect { isConnected ->
                _uiState.update { it.copy(isOffline = !isConnected) }
            }
        }
    }

    fun load(locale: Locale, showLoadingIndicator: Boolean = false) {
        viewModelScope.launch {
            val showLoading = showLoadingIndicator || !hasPresentedContent
            if (showLoading) {
                _uiState.update { it.copy(isLoading = true, error = null, errorMessage = null) }
            } else {
                _uiState.update { it.copy(error = null, errorMessage = null) }
            }
            runCatching {
                val readinessResult = getDataReadinessUseCase()
                val preferred = selectLocationUseCase.getPreferredLocation()
                val vehicles = listVehiclesUseCase()

                if (preferred == null || !readinessResult.hasCachedData) {
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            readiness = readinessResult.readiness,
                            hasCachedData = readinessResult.hasCachedData,
                            hasLocation = preferred != null,
                            municipality = preferred?.municipality,
                            state = preferred?.state,
                            surveyWeek = readinessResult.latestSurveyWeek,
                            prices = emptyList(),
                            tankFillCostEstimates = emptyList(),
                            isEmptyMunicipality = false,
                            dataAvailability = null,
                            operationalNote = null,
                        )
                    }
                    hasPresentedContent = true
                    return@launch
                }

                val pricesResult = getMunicipalityPricesUseCase()
                val tankFillResult = getTankFillCostEstimatesUseCase(
                    vehicles = vehicles,
                    state = pricesResult.state,
                    municipality = pricesResult.municipality,
                    surveyWeek = pricesResult.surveyWeek,
                )
                val uiPrices = com.anpfuel.app.mapper.AveragePriceUiMapper.toUiModels(
                    prices = pricesResult.prices,
                    locale = locale,
                )
                val uiTankFillEstimates = TankFillCostUiMapper.toUiModels(
                    items = tankFillResult.items,
                    locale = locale,
                    state = tankFillResult.state,
                    municipality = tankFillResult.municipality,
                )

                _uiState.update {
                    it.copy(
                        isLoading = false,
                        readiness = readinessResult.readiness,
                        hasCachedData = readinessResult.hasCachedData,
                        hasLocation = true,
                        municipality = pricesResult.municipality,
                        state = pricesResult.state,
                        surveyWeek = pricesResult.surveyWeek,
                        prices = uiPrices,
                        tankFillCostEstimates = uiTankFillEstimates,
                        isEmptyMunicipality = pricesResult.isEmpty,
                        dataAvailability = pricesResult.dataAvailability,
                        operationalNote = pricesResult.operationalNote,
                    )
                }
                hasPresentedContent = true
            }.onFailure { error ->
                val appError = AppErrorResolver.fromThrowable(error)
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        error = appError,
                        errorMessage = error.message ?: error.javaClass.simpleName,
                    )
                }
                hasPresentedContent = true
            }
        }
    }

    fun refresh(locale: Locale) {
        if (_uiState.value.isRefreshing) {
            return
        }

        viewModelScope.launch {
            _uiState.update { it.copy(isRefreshing = true, error = null, errorMessage = null) }
            runCatching {
                syncPriceTablesUseCase(SyncRequestSource.MANUAL)
            }.onFailure { error ->
                if (error !is DomainException) {
                    val appError = AppErrorResolver.fromThrowable(error)
                    _uiState.update {
                        it.copy(
                            isRefreshing = false,
                            error = appError,
                            errorMessage = error.message,
                        )
                    }
                    return@launch
                }
            }
            _uiState.update { it.copy(isRefreshing = false) }
            load(locale)
        }
    }
}
