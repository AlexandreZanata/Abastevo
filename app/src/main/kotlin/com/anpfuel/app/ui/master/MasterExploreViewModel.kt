package com.anpfuel.app.ui.master

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.application.usecase.ranking.GetPriceRankingUseCase
import com.anpfuel.domain.exploration.ChargingPlaceQuery
import com.anpfuel.domain.exploration.EnergyPriceQuery
import com.anpfuel.domain.exploration.InterpretationState
import com.anpfuel.domain.exploration.MasterQuestions
import com.anpfuel.domain.exploration.OfficialQuery
import com.anpfuel.domain.exploration.PriceQuery
import com.anpfuel.domain.exploration.ScenarioCostQuery
import com.anpfuel.domain.premium.PremiumValidationException
import dagger.hilt.android.lifecycle.HiltViewModel
import java.time.Instant
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** One human-readable interpretation line plus its machine kind. */
data class InterpretationView(
    val kind: String,
    val lines: List<String>,
    val state: String,
    val reason: String,
)

/** One tappable alternative that adopts a concrete typed query. */
data class AlternativeView(
    val label: String,
    val index: Int,
)

data class RankingRowView(
    val title: String,
    val subtitle: String,
)

data class MasterExploreUiState(
    val isLoading: Boolean = false,
    val question: String = "",
    val lastQuestion: String = "",
    val interpretation: InterpretationView? = null,
    val alternatives: List<AlternativeView> = emptyList(),
    val rows: List<RankingRowView> = emptyList(),
    val hasMore: Boolean = false,
    val isLoadingMore: Boolean = false,
    val revisionId: String? = null,
    val coverageNote: String? = null,
    val empty: Boolean = false,
    val route: String? = null,
    val isOffline: Boolean = false,
    val error: String? = null,
)

/**
 * MEX-31 master questions: interpret locally, show the typed result,
 * execute only price rankings (bounded, server-filtered) and route
 * official/EV intents to their existing surfaces. No guessing, no
 * phone-only subsets: ranking always hits the wire with the full
 * interpreted filter set.
 */
@HiltViewModel
class MasterExploreViewModel @Inject constructor(
    private val ranking: GetPriceRankingUseCase,
    private val connectivity: ObserveNetworkConnectivityUseCase,
) : ViewModel() {

    private val _uiState = MutableStateFlow(MasterExploreUiState())
    val uiState: StateFlow<MasterExploreUiState> = _uiState.asStateFlow()

    private var adopted: Any? = null
    private var lastPriceQuery: PriceQuery? = null
    private var lastCursor: String? = null

    init {
        viewModelScope.launch {
            runCatching {
                connectivity().collect { connected ->
                    _uiState.update { it.copy(isOffline = !connected) }
                }
            }
        }
    }

    fun onQuestionChanged(value: String) {
        _uiState.update { it.copy(question = value, error = null) }
    }

    fun askSuggestion(question: String) {
        _uiState.update { it.copy(question = question, error = null) }
        ask()
    }

    fun ask() {
        val question = _uiState.value.question
        if (question.isBlank()) return
        // Failed asks keep the previous interpretation on screen:
        // only a successful ask replaces context.
        _uiState.update {
            it.copy(
                isLoading = true, error = null, lastQuestion = question,
                revisionId = null, coverageNote = null,
            )
        }
        adopted = null
        viewModelScope.launch {
            runCatching {
                val now = Instant.now()
                val result = MasterQuestions.interpret(question, now)
                applyInterpretation(result.query, result.state.name, result.reason, result.alternatives, null)
            }.onFailure { error ->
                _uiState.update { it.copy(isLoading = false, error = error.message ?: error.javaClass.simpleName) }
            }
        }
    }

    fun chooseAlternative(index: Int) {
        val pending = pendingAlternatives ?: return
        val chosen = pending.getOrNull(index) ?: return
        adopted = chosen
        viewModelScope.launch {
            runCatching {
                applyInterpretation(chosen, InterpretationState.RESOLVED.name, "", emptyList(), null)
            }.onFailure { error ->
                _uiState.update { it.copy(isLoading = false, error = error.message ?: error.javaClass.simpleName) }
            }
        }
    }

    private var pendingAlternatives: List<Any>? = null

    private suspend fun applyInterpretation(query: Any?, state: String, reason: String, alternatives: List<Any>, cursor: String?) {
        pendingAlternatives = if (state == InterpretationState.NEEDS_CHOICE.name) alternatives else null
        when (state) {
            InterpretationState.UNSUPPORTED.name -> {
                // Refusals keep every held row and route: only the reason surfaces.
                _uiState.update { it.copy(error = reason) }
                return
            }
            InterpretationState.NEEDS_CHOICE.name -> {
                _uiState.update { it.copy(
                    interpretation = query?.let { describe(it) },
                    alternatives = alternatives.mapIndexed { index, alternative -> AlternativeView(labelOf(alternative), index) },
                    error = null,
                ) }
                return
            }
            else -> Unit
        }
        val view = query?.let { describe(it) }
        _uiState.update {
            it.copy(
                interpretation = view,
                alternatives = emptyList(),
                rows = emptyList(),
                route = null,
                error = null,
            )
        }
        when {
            state == InterpretationState.NEEDS_CHOICE.name -> _uiState.update { it.copy(isLoading = false) }
            state == InterpretationState.UNSUPPORTED.name -> _uiState.update { it.copy(isLoading = false) }
            query is PriceQuery -> executeRanking(query, cursor)
            query is OfficialQuery -> _uiState.update { it.copy(isLoading = false, route = "OFFICIAL") }
            query is ChargingPlaceQuery -> _uiState.update { it.copy(isLoading = false, route = "CHARGING:" + query.connector) }
            query is EnergyPriceQuery -> _uiState.update { it.copy(isLoading = false, route = "ENERGY") }
            query is com.anpfuel.domain.exploration.ScenarioCostQuery -> _uiState.update { it.copy(isLoading = false, route = "SCENARIO") }
            else -> _uiState.update { it.copy(isLoading = false) }
        }
    }

    private suspend fun executeRanking(query: PriceQuery, cursor: String?) {
        lastPriceQuery = query
        try {
            val page = ranking(query, cursor)
            lastCursor = page.nextCursor
            val rows = page.stations.map {
                RankingRowView(
                    title = it.amountMilliBrl.toString(),
                    subtitle = listOfNotNull(it.state, it.collectedOn).joinToString(" · "),
                )
            }
            _uiState.update {
                it.copy(
                    isLoading = false, rows = rows, revisionId = page.revisionId,
                    coverageNote = page.generatedAt, empty = rows.isEmpty() && page.areas.isEmpty(),
                    hasMore = page.nextCursor != null,
                )
            }
        } catch (e: Exception) {
            if (e is PremiumValidationException) {
                _uiState.update { it.copy(isLoading = false, error = e.code) }
            } else {
                _uiState.update { it.copy(isLoading = false, isOffline = true, error = e.message) }
            }
        }
    }

    fun loadMore() {
        val query = lastPriceQuery ?: return
        val cursor = lastCursor ?: return
        if (_uiState.value.isLoadingMore) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoadingMore = true) }
            runCatching {
                val page = ranking(query, cursor)
                lastCursor = page.nextCursor
                val more = page.stations.map {
                    RankingRowView(
                        title = it.amountMilliBrl.toString(),
                        subtitle = listOfNotNull(it.state, it.collectedOn).joinToString(" · "),
                    )
                }
                _uiState.update { it.copy(isLoadingMore = false, rows = it.rows + more, hasMore = page.nextCursor != null) }
            }.onFailure { error ->
                _uiState.update { it.copy(isLoadingMore = false, error = error.message ?: error.javaClass.simpleName) }
            }
        }
    }

    fun dismissError() {
        _uiState.update { it.copy(error = null) }
    }

    private fun describe(query: Any): InterpretationView = when (query) {
        is PriceQuery -> InterpretationView(
            kind = query.kind,
            lines = listOf(
                query.product.fuel, query.product.unit,
                query.condition.kind.ifEmpty { "STANDARD" },
                query.geography.level + query.geography.uf.takeIf { it.isNotEmpty() }?.let { " $it" }.orEmpty(),
                query.metric.kind + "/" + query.metric.groupBy,
                query.page.limit.toString(),
            ),
            state = InterpretationState.RESOLVED.name,
            reason = "",
        )
        is OfficialQuery -> InterpretationView(
            kind = query.kind,
            lines = query.sources + query.recordKinds + query.status.kind,
            state = InterpretationState.RESOLVED.name,
            reason = "",
        )
        is ChargingPlaceQuery -> InterpretationView(
            kind = query.kind,
            lines = listOf(
                query.profile,
                query.connector.ifEmpty { "any connector" },
                (query.current.ifEmpty { "any current" }),
            ),
            state = InterpretationState.RESOLVED.name,
            reason = "",
        )
        is EnergyPriceQuery -> InterpretationView(
            kind = query.kind,
            lines = listOf(query.profile, query.sort),
            state = InterpretationState.RESOLVED.name,
            reason = "",
        )
        is ScenarioCostQuery -> InterpretationView(
            kind = query.kind,
            lines = query.dimensions.sorted() + "missing:" + query.missing.sorted().joinToString(","),
            state = InterpretationState.RESOLVED.name,
            reason = "",
        )
        else -> InterpretationView(kind = "UNKNOWN", lines = emptyList(), state = InterpretationState.UNSUPPORTED.name, reason = "unknown")
    }

    private fun labelOf(alternative: Any): String = when (alternative) {
        is PriceQuery -> alternative.product.fuel + " " + alternative.metric.kind
        is OfficialQuery -> alternative.sources.joinToString(",")
        is ChargingPlaceQuery -> alternative.connector.ifEmpty { "any connector" }
        is EnergyPriceQuery -> "energy price " + alternative.sort
        is ScenarioCostQuery -> alternative.dimensions.sorted().joinToString("+")
        else -> alternative.toString()
    }
}
