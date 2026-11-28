package com.anpfuel.app.ui.weekpicker

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.readiness.GetDataReadinessUseCase
import com.anpfuel.application.usecase.settings.GetSettingsUseCase
import com.anpfuel.domain.state.DataReadinessState
import com.anpfuel.domain.valueobject.SurveyWeek
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class ActiveSurveyWeekUiState(
    val surveyWeek: SurveyWeek? = null,
    val isLatest: Boolean = false,
    val isStale: Boolean = false,
    /** False until the first read finishes; the trigger shows a same-size placeholder meanwhile. */
    val isLoaded: Boolean = false,
)

@HiltViewModel
class ActiveSurveyWeekViewModel @Inject constructor(
    private val getSettingsUseCase: GetSettingsUseCase,
    private val getDataReadinessUseCase: GetDataReadinessUseCase,
) : ViewModel() {

    private val _uiState = MutableStateFlow(ActiveSurveyWeekUiState())
    val uiState: StateFlow<ActiveSurveyWeekUiState> = _uiState.asStateFlow()

    fun load() {
        viewModelScope.launch {
            try {
                val preferences = getSettingsUseCase()
                val readiness = getDataReadinessUseCase()
                val activeWeek = preferences.activeSurveyWeek
                _uiState.update {
                    it.copy(
                        surveyWeek = activeWeek,
                        isLatest = activeWeek != null && activeWeek == readiness.latestSurveyWeek,
                        isStale = readiness.readiness == DataReadinessState.STALE,
                        isLoaded = true,
                    )
                }
            } finally {
                // A failed read ends the placeholder too; it never shimmers forever.
                _uiState.update { it.copy(isLoaded = true) }
            }
        }
    }
}
