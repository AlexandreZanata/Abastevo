package com.anpfuel.app.ui.map

import com.anpfuel.domain.routing.NavigationProgress
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

data class GuidanceUiState(
    val active: Boolean = false,
    val owner: String = "",
    val progress: NavigationProgress? = null,
    val notifications: List<String> = emptyList(),
    val recalculating: Boolean = false,
    val rerouteFailed: Boolean = false,
    val gpsPoor: Boolean = false,
    val gpsLost: Boolean = false,
    val voiceUnavailable: Boolean = false,
    val error: String? = null,
    val resumeAvailable: Boolean = false,
)

/** Process-visible guidance state; the service is the single owner. */
object GuidanceServiceState {
    private val mutable = MutableStateFlow(GuidanceUiState())
    val flow: StateFlow<GuidanceUiState> = mutable.asStateFlow()
    fun update(transform: (GuidanceUiState) -> GuidanceUiState) {
        mutable.value = transform(mutable.value)
    }
    fun current(): GuidanceUiState = mutable.value
}
