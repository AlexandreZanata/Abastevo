package com.anpfuel.app.ui.monthly

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.ledger.GetMonthlyIndicatorsUseCase
import com.anpfuel.application.usecase.ledger.MonthlyIndicatorsCsv
import com.anpfuel.application.usecase.ledger.MonthlyVehicleIndicators
import com.anpfuel.application.usecase.vehicle.GetVehicleUseCase
import com.anpfuel.domain.valueobject.DomainId
import dagger.hilt.android.lifecycle.HiltViewModel
import java.time.ZoneId
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class MonthlyIndicatorsUiState(
    val isLoading: Boolean = true,
    val vehicleId: DomainId? = null,
    val vehicleName: String = "",
    val year: Int = 2026,
    val month: Int = 1,
    val rows: List<MonthlyVehicleIndicators> = emptyList(),
    val exportCsv: String? = null,
    val errorMessage: String? = null,
)

@HiltViewModel
class MonthlyIndicatorsViewModel @Inject constructor(
    private val indicators: GetMonthlyIndicatorsUseCase,
    private val getVehicle: GetVehicleUseCase,
) : ViewModel() {

    private val _uiState = MutableStateFlow(MonthlyIndicatorsUiState())
    val uiState: StateFlow<MonthlyIndicatorsUiState> = _uiState.asStateFlow()

    fun load(vehicleId: DomainId, year: Int, month: Int, zone: ZoneId) {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, errorMessage = null, vehicleId = vehicleId, year = year, month = month) }
            runCatching {
                val vehicle = getVehicle(vehicleId)
                val rows = indicators(vehicleId, year, month, zone.id)
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        vehicleName = vehicle?.displayName.orEmpty(),
                        rows = rows,
                    )
                }
            }.onFailure { error ->
                _uiState.update { it.copy(isLoading = false, errorMessage = error.message) }
            }
        }
    }

    fun reload() {
        val state = _uiState.value
        val id = state.vehicleId ?: return
        load(id, state.year, state.month, ZoneId.systemDefault())
    }

    fun nextMonth() {
        val state = _uiState.value
        val id = state.vehicleId ?: return
        if (state.month == 12) load(id, state.year + 1, 1, ZoneId.systemDefault())
        else load(id, state.year, state.month + 1, ZoneId.systemDefault())
    }

    fun previousMonth() {
        val state = _uiState.value
        val id = state.vehicleId ?: return
        if (state.month == 1) {
            if (state.year > 2000) load(id, state.year - 1, 12, ZoneId.systemDefault())
        } else {
            load(id, state.year, state.month - 1, ZoneId.systemDefault())
        }
    }

    fun prepareExport() {
        val state = _uiState.value
        if (state.rows.isEmpty()) return
        _uiState.update {
            it.copy(exportCsv = MonthlyIndicatorsCsv.build(it.vehicleName, it.rows))
        }
    }

    fun onExportConsumed() {
        _uiState.update { it.copy(exportCsv = null) }
    }

    fun dismissError() {
        _uiState.update { it.copy(errorMessage = null) }
    }
}
