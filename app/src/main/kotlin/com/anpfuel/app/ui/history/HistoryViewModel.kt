package com.anpfuel.app.ui.history

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.application.usecase.price.GetPriceHistoryUseCase
import com.anpfuel.application.usecase.price.PriceHistoryOutcome
import com.anpfuel.app.mapper.HistoryEntryUiMapper
import com.anpfuel.app.ui.model.HistoryEntryUiModel
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import dagger.hilt.android.lifecycle.HiltViewModel
import java.util.Locale
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class HistoryUiState(
    val isLoading: Boolean = true,
    val isOffline: Boolean = false,
    val selectedFuelProduct: FuelProduct = FuelProduct.GASOLINE_REGULAR,
    val municipality: String? = null,
    val state: BrazilianState? = null,
    val entries: List<HistoryEntryUiModel> = emptyList(),
    val showInsufficientData: Boolean = false,
    val showHistoryDisabled: Boolean = false,
    val errorMessage: String? = null,
)

@HiltViewModel
class HistoryViewModel @Inject constructor(
    private val getPriceHistoryUseCase: GetPriceHistoryUseCase,
    observeNetworkConnectivityUseCase: ObserveNetworkConnectivityUseCase,
) : ViewModel() {

    private val _uiState = MutableStateFlow(HistoryUiState())
    val uiState: StateFlow<HistoryUiState> = _uiState.asStateFlow()

    init {
        viewModelScope.launch {
            observeNetworkConnectivityUseCase().collect { isConnected ->
                _uiState.update { it.copy(isOffline = !isConnected) }
            }
        }
    }

    fun load(locale: Locale) {
        loadForFuel(_uiState.value.selectedFuelProduct, locale)
    }

    fun onFuelProductSelected(fuelProduct: FuelProduct, locale: Locale) {
        if (_uiState.value.selectedFuelProduct == fuelProduct) {
            return
        }
        _uiState.update { it.copy(selectedFuelProduct = fuelProduct) }
        loadForFuel(fuelProduct, locale)
    }

    private fun loadForFuel(fuelProduct: FuelProduct, locale: Locale) {
        viewModelScope.launch {
            _uiState.update {
                it.copy(
                    isLoading = true,
                    errorMessage = null,
                    showInsufficientData = false,
                    showHistoryDisabled = false,
                    entries = emptyList(),
                )
            }

            runCatching {
                when (val outcome = getPriceHistoryUseCase(fuelProduct = fuelProduct)) {
                    PriceHistoryOutcome.HistoryDisabled -> {
                        _uiState.update {
                            it.copy(
                                isLoading = false,
                                showHistoryDisabled = true,
                            )
                        }
                    }

                    PriceHistoryOutcome.InsufficientData -> {
                        _uiState.update {
                            it.copy(
                                isLoading = false,
                                showInsufficientData = true,
                            )
                        }
                    }

                    is PriceHistoryOutcome.Success -> {
                        _uiState.update {
                            it.copy(
                                isLoading = false,
                                municipality = outcome.municipality,
                                state = outcome.state,
                                selectedFuelProduct = outcome.fuelProduct,
                                entries = HistoryEntryUiMapper.toUiModels(
                                    entries = outcome.entries,
                                    locale = locale,
                                ),
                            )
                        }
                    }
                }
            }.onFailure { error ->
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        errorMessage = error.message ?: error.javaClass.simpleName,
                    )
                }
            }
        }
    }
}
