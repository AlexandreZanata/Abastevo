package com.anpfuel.app.ui.search

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.error.AppError
import com.anpfuel.application.usecase.location.SearchMunicipalityOutcome
import com.anpfuel.application.usecase.location.SearchMunicipalityUseCase
import com.anpfuel.application.usecase.location.SelectLocationUseCase
import com.anpfuel.domain.model.MunicipalitySearchResult
import com.anpfuel.domain.rule.MinimumSearchLengthRule
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.debounce
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.launchIn
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class SearchUiState(
    val query: String = "",
    val isSearching: Boolean = false,
    val isSaving: Boolean = false,
    val results: List<MunicipalitySearchResult> = emptyList(),
    val showMinCharsHint: Boolean = false,
    val showNoResults: Boolean = false,
    val error: AppError? = null,
    val errorMessage: String? = null,
)

sealed interface SearchNavigation {
    data object ToHome : SearchNavigation
    data object ToLocationPicker : SearchNavigation
}

@OptIn(FlowPreview::class)
@HiltViewModel
class SearchViewModel @Inject constructor(
    private val searchMunicipalityUseCase: SearchMunicipalityUseCase,
    private val selectLocationUseCase: SelectLocationUseCase,
) : ViewModel() {

    private val _uiState = MutableStateFlow(SearchUiState())
    val uiState: StateFlow<SearchUiState> = _uiState.asStateFlow()

    private val _navigation = MutableSharedFlow<SearchNavigation>(extraBufferCapacity = 1)
    val navigation: SharedFlow<SearchNavigation> = _navigation.asSharedFlow()

    private val queryChanges = MutableStateFlow("")

    init {
        queryChanges
            .debounce(SEARCH_DEBOUNCE_MS)
            .distinctUntilChanged()
            .onEach { query -> executeSearch(query) }
            .launchIn(viewModelScope)
    }

    fun onQueryChange(query: String) {
        _uiState.update {
            it.copy(
                query = query,
                showMinCharsHint = query.trim().length < MinimumSearchLengthRule.MIN_LENGTH,
                showNoResults = false,
                error = null,
                errorMessage = null,
            )
        }
        queryChanges.value = query
    }

    fun onResultSelected(result: MunicipalitySearchResult) {
        if (_uiState.value.isSaving) {
            return
        }

        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, errorMessage = null) }
            runCatching {
                selectLocationUseCase.selectLocation(
                    state = result.state,
                    municipality = result.municipality,
                )
                _uiState.update { it.copy(isSaving = false) }
                _navigation.emit(SearchNavigation.ToHome)
            }.onFailure { error ->
                _uiState.update {
                    it.copy(
                        isSaving = false,
                        errorMessage = error.message ?: error.javaClass.simpleName,
                    )
                }
            }
        }
    }

    fun onBrowseByState() {
        viewModelScope.launch {
            _navigation.emit(SearchNavigation.ToLocationPicker)
        }
    }

    private suspend fun executeSearch(query: String) {
        val trimmed = query.trim()
        if (trimmed.length < MinimumSearchLengthRule.MIN_LENGTH) {
            _uiState.update {
                it.copy(
                    isSearching = false,
                    results = emptyList(),
                    showNoResults = false,
                    error = null,
                )
            }
            return
        }

        _uiState.update { it.copy(isSearching = true, showNoResults = false, error = null) }

        when (val outcome = searchMunicipalityUseCase.search(trimmed)) {
            SearchMunicipalityOutcome.QueryTooShort -> {
                _uiState.update {
                    it.copy(
                        isSearching = false,
                        results = emptyList(),
                        showMinCharsHint = true,
                        showNoResults = false,
                    )
                }
            }

            is SearchMunicipalityOutcome.NoResults -> {
                _uiState.update {
                    it.copy(
                        isSearching = false,
                        results = emptyList(),
                        showNoResults = true,
                        error = outcome.error,
                    )
                }
            }

            is SearchMunicipalityOutcome.Success -> {
                _uiState.update {
                    it.copy(
                        isSearching = false,
                        results = outcome.results,
                        showNoResults = false,
                        error = null,
                    )
                }
            }
        }
    }

    companion object {
        private const val SEARCH_DEBOUNCE_MS = 300L
    }
}
