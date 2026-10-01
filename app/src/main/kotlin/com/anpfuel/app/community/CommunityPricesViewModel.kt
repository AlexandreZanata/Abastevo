package com.anpfuel.app.community

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.port.CommunityReadsFlagProvider
import com.anpfuel.application.usecase.community.CommunityPriceGroupsOutcome
import com.anpfuel.application.usecase.community.GetCommunityPriceGroupsUseCase
import com.anpfuel.domain.valueobject.FuelProduct
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/**
 * P10-T06 community price state (B-BR-001/008, MIGRATION_PLAN).
 *
 * Flag-gated: [UiState.Disabled] performs no network/cache IO and the
 * caller keeps the local ANP path (rollback is flag OFF). Otherwise one
 * station fetch resolves to [UiState.Content] (fresh, community UNKNOWN
 * until P04), [UiState.Stale] (explicit stale fallback, never as fresh)
 * or [UiState.Unavailable] (retry). Confidence never renders as a
 * guarantee; units/conditions/sources stay on every row.
 */
sealed interface CommunityPricesUiState {
    data object Disabled : CommunityPricesUiState
    data object Loading : CommunityPricesUiState
    data class Content(
        val rows: List<CommunityPriceRowUiModel>,
        val source: String,
        val version: String,
    ) : CommunityPricesUiState

    data class Stale(
        val rows: List<CommunityPriceRowUiModel>,
        val source: String,
        val version: String,
    ) : CommunityPricesUiState

    data class Unavailable(val message: String) : CommunityPricesUiState
}

@HiltViewModel
class CommunityPricesViewModel @Inject constructor(
    private val useCase: GetCommunityPriceGroupsUseCase,
    private val flagProvider: CommunityReadsFlagProvider,
) : ViewModel() {

    private val _state =
        MutableStateFlow<CommunityPricesUiState>(initialState())
    val state: StateFlow<CommunityPricesUiState> = _state.asStateFlow()

    fun load(stationId: String, fuelProduct: FuelProduct? = null) {
        if (!flagProvider.isEnabled()) {
            _state.value = CommunityPricesUiState.Disabled
            return
        }
        viewModelScope.launch {
            _state.value = CommunityPricesUiState.Loading
            _state.value = try {
                when (val outcome = useCase(stationId, fuelProduct)) {
                    is CommunityPriceGroupsOutcome.Disabled ->
                        CommunityPricesUiState.Disabled
                    is CommunityPriceGroupsOutcome.Fresh -> CommunityPricesUiState.Content(
                        rows = CommunityPriceDisplay.fromGroups(outcome.groups),
                        source = outcome.groups.source,
                        version = outcome.groups.version,
                    )
                    is CommunityPriceGroupsOutcome.StaleCache -> CommunityPricesUiState.Stale(
                        rows = CommunityPriceDisplay.fromGroups(outcome.groups, staleCache = true),
                        source = outcome.groups.source,
                        version = outcome.groups.version,
                    )
                    is CommunityPriceGroupsOutcome.Unavailable ->
                        CommunityPricesUiState.Unavailable(
                            outcome.cause.message ?: outcome.cause.javaClass.simpleName,
                        )
                }
            } catch (error: Exception) {
                CommunityPricesUiState.Unavailable(
                    error.message ?: error.javaClass.simpleName,
                )
            }
        }
    }

    private fun initialState(): CommunityPricesUiState =
        if (!flagProvider.isEnabled()) CommunityPricesUiState.Disabled
        else CommunityPricesUiState.Loading
}
