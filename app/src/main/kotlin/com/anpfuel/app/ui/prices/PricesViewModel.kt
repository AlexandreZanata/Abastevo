package com.anpfuel.app.ui.prices

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.error.AppError
import com.anpfuel.application.error.AppErrorResolver
import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.application.usecase.price.GetMunicipalityPricesUseCase
import com.anpfuel.application.usecase.readiness.GetDataReadinessUseCase
import com.anpfuel.app.ui.model.AveragePriceUiModel
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

data class PricesUiState(
    val isLoading: Boolean = true,
    val readiness: DataReadinessState = DataReadinessState.EMPTY,
    val isOffline: Boolean = false,
    val municipality: String? = null,
    val state: BrazilianState? = null,
    val surveyWeek: SurveyWeek? = null,
    val prices: List<AveragePriceUiModel> = emptyList(),
    val isEmptyMunicipality: Boolean = false,
    val dataAvailability: DataAvailability? = null,
    val operationalNote: String? = null,
    val error: AppError? = null,
    val errorMessage: String? = null,
)

@HiltViewModel
class PricesViewModel @Inject constructor(
    private val getDataReadinessUseCase: GetDataReadinessUseCase,
    private val getMunicipalityPricesUseCase: GetMunicipalityPricesUseCase,
    observeNetworkConnectivityUseCase: ObserveNetworkConnectivityUseCase,
) : ViewModel() {

    private val _uiState = MutableStateFlow(PricesUiState())
    val uiState: StateFlow<PricesUiState> = _uiState.asStateFlow()

    init {
        viewModelScope.launch {
            observeNetworkConnectivityUseCase().collect { isConnected ->
                _uiState.update { it.copy(isOffline = !isConnected) }
            }
        }
    }

    fun load(locale: Locale) {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorMessage = null) }
            runCatching {
                val readinessResult = getDataReadinessUseCase()
                val pricesResult = getMunicipalityPricesUseCase()
                val uiPrices = com.anpfuel.app.mapper.AveragePriceUiMapper.toUiModels(
                    prices = pricesResult.prices,
                    locale = locale,
                )

                _uiState.update {
                    it.copy(
                        isLoading = false,
                        readiness = readinessResult.readiness,
                        municipality = pricesResult.municipality,
                        state = pricesResult.state,
                        surveyWeek = pricesResult.surveyWeek,
                        prices = uiPrices,
                        isEmptyMunicipality = pricesResult.isEmpty,
                        dataAvailability = pricesResult.dataAvailability,
                        operationalNote = pricesResult.operationalNote,
                    )
                }
            }.onFailure { error ->
                val appError = AppErrorResolver.fromThrowable(error)
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        error = appError,
                        errorMessage = error.message ?: error.javaClass.simpleName,
                    )
                }
            }
        }
    }
}
