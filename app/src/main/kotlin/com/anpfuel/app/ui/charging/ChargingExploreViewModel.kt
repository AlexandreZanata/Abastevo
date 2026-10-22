package com.anpfuel.app.ui.charging

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.application.usecase.vehicle.GetVehicleUseCase
import com.anpfuel.data.repository.ChargingDiscoveryRepository
import com.anpfuel.data.repository.DiscoveryRow
import com.anpfuel.data.repository.PlaceDetail
import com.anpfuel.domain.charging.ChargingAccess
import com.anpfuel.domain.charging.ChargingCompatibility
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingCurrent
import com.anpfuel.domain.charging.Compatibility
import com.anpfuel.domain.valueobject.DomainId
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class ChargingExploreUiState(
    val isLoading: Boolean = true,
    val isRefreshing: Boolean = false,
    val isLoadingMore: Boolean = false,
    val isOffline: Boolean = false,
    val vehicleId: DomainId? = null,
    val vehicleName: String = "",
    val vehicleConnector: ChargingConnector? = null,
    val connector: ChargingConnector? = null,
    val current: ChargingCurrent? = null,
    val access: ChargingAccess? = null,
    val rows: List<DiscoveryRow> = emptyList(),
    val hasMore: Boolean = false,
    val detail: PlaceDetail? = null,
    val sourceRevision: String? = null,
    val sourceSavedAt: Long? = null,
    val errorMessage: String? = null,
)

@HiltViewModel
class ChargingExploreViewModel @Inject constructor(
    private val repository: ChargingDiscoveryRepository,
    private val getVehicle: GetVehicleUseCase,
    private val observeConnectivity: ObserveNetworkConnectivityUseCase,
) : ViewModel() {

    private val _uiState = MutableStateFlow(ChargingExploreUiState())
    val uiState: StateFlow<ChargingExploreUiState> = _uiState.asStateFlow()

    private var generation: Long? = null
    private var offset = 0

    fun load(vehicleId: DomainId) {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, errorMessage = null, vehicleId = vehicleId) }
            runCatching {
                val vehicle = getVehicle(vehicleId)
                val connector = vehicle?.chargeConnector
                _uiState.update {
                    it.copy(
                        vehicleName = vehicle?.displayName.orEmpty(),
                        vehicleConnector = connector,
                        connector = connector,
                    )
                }
                observeConnectivity().collect { connected ->
                    _uiState.update { it.copy(isOffline = !connected) }
                }
            }.onFailure { error ->
                _uiState.update { it.copy(isLoading = false, errorMessage = error.message) }
            }
        }
        viewModelScope.launch {
            runCatching {
                val meta = repository.generationMeta(PROFILE)
                if (meta == null) {
                    refreshInternal()
                } else {
                    generation = meta.generation
                    _uiState.update { it.copy(sourceRevision = meta.sourceRevision, sourceSavedAt = meta.savedAtMillis) }
                    appendPage(reset = true)
                    _uiState.update { it.copy(isLoading = false) }
                }
            }.onFailure { error ->
                _uiState.update { it.copy(isLoading = false, errorMessage = error.message) }
            }
        }
    }

    fun refresh() {
        viewModelScope.launch {
            _uiState.update { it.copy(isRefreshing = true, errorMessage = null) }
            runCatching {
                refreshInternal()
            }.onFailure { error ->
                _uiState.update { it.copy(errorMessage = error.message, isOffline = true) }
            }
            _uiState.update { it.copy(isRefreshing = false) }
        }
    }

    private suspend fun refreshInternal() {
        val meta = repository.refresh(PROFILE)
        generation = meta.generation
        _uiState.update { it.copy(sourceRevision = meta.sourceRevision, sourceSavedAt = meta.savedAtMillis) }
        appendPage(reset = true)
        _uiState.update { it.copy(isLoading = false) }
    }

    fun loadMore() {
        val gen = generation ?: return
        if (_uiState.value.isLoadingMore || !_uiState.value.hasMore) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoadingMore = true) }
            runCatching {
                offset += PAGE_SIZE
                appendPage(reset = false, generation = gen)
            }.onFailure { error ->
                offset -= PAGE_SIZE
                _uiState.update { it.copy(errorMessage = error.message) }
            }
            _uiState.update { it.copy(isLoadingMore = false) }
        }
    }

    private suspend fun appendPage(reset: Boolean, generation: Long? = this.generation) {
        val gen = generation ?: return
        if (reset) offset = 0
        val state = _uiState.value
        val page = repository.list(
            profile = PROFILE, generation = gen, connector = state.connector,
            current = state.current, access = state.access, limit = PAGE_SIZE, offset = offset,
        ).map { row ->
            // Verdicts follow the selected filters: an explicit connector
            // filter re-assesses every row, otherwise the vehicle answers.
            val verdict = ChargingCompatibility.assess(state.connector ?: state.vehicleConnector, row.place.groups)
            row.copy(verdict = verdict)
        }
        val merged = if (reset) page else (_uiState.value.rows + page).distinctBy { it.place.id }
        _uiState.update {
            it.copy(
                rows = merged,
                hasMore = page.size >= PAGE_SIZE,
            )
        }
    }

    fun onConnectorChanged(connector: ChargingConnector?) {
        _uiState.update { it.copy(connector = connector) }
        reloadWindow()
    }

    fun onCurrentChanged(current: ChargingCurrent?) {
        _uiState.update { it.copy(current = current) }
        reloadWindow()
    }

    fun onAccessChanged(access: ChargingAccess?) {
        _uiState.update { it.copy(access = access) }
        reloadWindow()
    }

    private fun reloadWindow() {
        val gen = generation ?: return
        viewModelScope.launch {
            runCatching {
                offset = 0
                appendPage(reset = true, generation = gen)
            }.onFailure { error ->
                _uiState.update { it.copy(errorMessage = error.message) }
            }
        }
    }

    fun openDetail(placeId: String) {
        val gen = generation ?: return
        viewModelScope.launch {
            runCatching {
                _uiState.update { it.copy(detail = repository.detail(placeId, PROFILE, gen)) }
            }.onFailure { error ->
                _uiState.update { it.copy(errorMessage = error.message) }
            }
        }
    }

    fun dismissDetail() {
        _uiState.update { it.copy(detail = null) }
    }

    fun dismissError() {
        _uiState.update { it.copy(errorMessage = null) }
    }

    companion object {
        const val PROFILE = "ocm-br-20261021-v1"
        const val PAGE_SIZE = 20
    }
}
