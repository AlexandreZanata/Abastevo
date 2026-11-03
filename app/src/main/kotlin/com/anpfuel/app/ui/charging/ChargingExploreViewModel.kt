package com.anpfuel.app.ui.charging

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.application.usecase.vehicle.GetVehicleUseCase
import com.anpfuel.data.local.charging.StoredPackMeta
import com.anpfuel.data.remote.charging.ChargingSnapshotUnavailable
import com.anpfuel.data.repository.ChargingDiscoveryRepository
import com.anpfuel.data.repository.DiscoveryRow
import com.anpfuel.data.repository.PlaceDetail
import com.anpfuel.domain.charging.ChargingAccess
import com.anpfuel.domain.charging.ChargingCompatibility
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingCurrent
import com.anpfuel.domain.charging.ChargingSnapshotAge
import com.anpfuel.domain.charging.ChargingSnapshotFreshness
import com.anpfuel.domain.charging.Compatibility
import com.anpfuel.domain.valueobject.DomainId
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.Job
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
    val minPowerW: Long? = null,
    val profile: String = ChargingDiscoveryRepository.OCM_PROFILE,
    val rows: List<DiscoveryRow> = emptyList(),
    val hasMore: Boolean = false,
    val detail: PlaceDetail? = null,
    val sourceRevision: String? = null,
    val sourceSavedAt: Long? = null,
    val sourceAge: ChargingSnapshotAge = ChargingSnapshotAge.UNKNOWN,
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
    private var nextOffset: Int? = null
    private var windowRequest = 0
    private var refreshJob: Job? = null

    fun load(vehicleId: DomainId) {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, errorMessage = null, vehicleId = vehicleId) }
            runCatching {
                val vehicle = getVehicle(vehicleId)
                val connector = vehicle?.chargeConnector
                val filterChanged = _uiState.value.connector != connector
                _uiState.update {
                    it.copy(
                        vehicleName = vehicle?.displayName.orEmpty(),
                        vehicleConnector = connector,
                        connector = connector,
                    )
                }
                if (filterChanged) reloadWindow()
                observeConnectivity().collect { connected ->
                    _uiState.update { it.copy(isOffline = !connected) }
                }
            }.onFailure { error ->
                _uiState.update { it.copy(isLoading = false, errorMessage = error.message) }
            }
        }
        viewModelScope.launch {
            runCatching {
                val cached = repository.generationMeta(ChargingDiscoveryRepository.NATIONAL_PROFILE)
                    ?: repository.generationMeta(ChargingDiscoveryRepository.OCM_PROFILE)
                if (cached == null) refreshInternal() else showGeneration(cached)
            }.onFailure { error ->
                _uiState.update { it.copy(isLoading = false, errorMessage = error.message) }
            }
        }
    }

    /**
     * MEX-32/EE-26 single-flight manual refresh: repeated triggers while
     * one equivalent pack read is active converge on that read instead
     * of restarting it. The previously loaded window is kept until the
     * replacement generation is committed.
     */
    fun refresh() {
        if (refreshJob?.isActive == true) return
        refreshJob = viewModelScope.launch {
            _uiState.update { it.copy(isRefreshing = true, errorMessage = null) }
            runCatching {
                refreshInternal()
            }.onFailure { error ->
                _uiState.update { it.copy(isLoading = false, errorMessage = error.message, isOffline = true) }
            }
            _uiState.update { it.copy(isRefreshing = false) }
        }
    }

    private suspend fun refreshInternal() {
        val meta = try {
            repository.refresh(ChargingDiscoveryRepository.NATIONAL_PROFILE)
        } catch (unavailable: ChargingSnapshotUnavailable) {
            if (_uiState.value.profile == ChargingDiscoveryRepository.NATIONAL_PROFILE) throw unavailable
            repository.refresh(ChargingDiscoveryRepository.OCM_PROFILE)
        }
        showGeneration(meta)
    }

    private suspend fun showGeneration(meta: StoredPackMeta) {
        generation = meta.generation
        nextOffset = null
        _uiState.update {
            it.copy(
                profile = meta.profile,
                rows = emptyList(),
                detail = null,
                hasMore = false,
                isLoading = true,
                sourceRevision = meta.sourceRevision,
                sourceSavedAt = meta.savedAtMillis,
                sourceAge = ChargingSnapshotFreshness.classify(System.currentTimeMillis(), meta.savedAtMillis),
            )
        }
        appendPage(reset = true)
        _uiState.update { it.copy(isLoading = false) }
    }

    fun loadMore() {
        val gen = generation ?: return
        if (_uiState.value.isLoadingMore || nextOffset == null) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoadingMore = true) }
            runCatching {
                appendPage(reset = false, generation = gen)
            }.onFailure { error ->
                _uiState.update { it.copy(errorMessage = error.message) }
            }
            _uiState.update { it.copy(isLoadingMore = false) }
        }
    }

    private suspend fun appendPage(reset: Boolean, generation: Long? = this.generation) {
        val gen = generation ?: return
        val offset = if (reset) 0 else nextOffset ?: return
        val request = ++windowRequest
        val state = _uiState.value
        val result = repository.explorePage(
            profile = state.profile, generation = gen, connector = state.connector,
            current = state.current, access = state.access, minPowerW = state.minPowerW,
            limit = PAGE_SIZE, offset = offset,
        )
        if (request != windowRequest || gen != this.generation) return
        nextOffset = result.nextOffset
        val page = result.rows.map { row ->
            // Verdicts follow the selected filters: an explicit connector
            // filter re-assesses every row, otherwise the vehicle answers.
            val verdict = ChargingCompatibility.assess(state.connector ?: state.vehicleConnector, row.place.groups)
            row.copy(verdict = verdict)
        }
        val merged = if (reset) page else (_uiState.value.rows + page).distinctBy { it.place.id }
        _uiState.update {
            it.copy(
                rows = merged,
                hasMore = nextOffset != null,
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

    fun onMinPowerChanged(minPowerW: Long?) {
        _uiState.update { it.copy(minPowerW = minPowerW) }
        reloadWindow()
    }

    private fun reloadWindow() {
        val gen = generation ?: return
        ++windowRequest
        nextOffset = null
        _uiState.update { it.copy(hasMore = false) }
        viewModelScope.launch {
            runCatching {
                appendPage(reset = true, generation = gen)
            }.onFailure { error ->
                _uiState.update { it.copy(errorMessage = error.message) }
            }
        }
    }

    fun openDetail(placeId: String) {
        val gen = generation ?: return
        val profile = _uiState.value.profile
        viewModelScope.launch {
            runCatching {
                val detail = repository.detail(placeId, profile, gen)
                if (gen == generation && profile == _uiState.value.profile) {
                    _uiState.update { it.copy(detail = detail) }
                }
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
        const val PAGE_SIZE = 20
    }
}
