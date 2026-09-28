package com.anpfuel.app.ui.weekpicker

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.readiness.GetDataReadinessUseCase
import com.anpfuel.application.usecase.settings.GetSettingsUseCase
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
            val preferences = getSettingsUseCase()
            val readiness = getDataReadinessUseCase()
            val activeWeek = preferences.activeSurveyWeek
            _uiState.update {
                it.copy(
                    surveyWeek = activeWeek,
                    isLatest = activeWeek != null && activeWeek == readiness.latestSurveyWeek,
                )
            }
        }
    }
}
