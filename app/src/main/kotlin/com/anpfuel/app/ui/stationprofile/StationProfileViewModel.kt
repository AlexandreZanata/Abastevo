package com.anpfuel.app.ui.stationprofile

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.profile.GetStationProfileUseCase
import com.anpfuel.application.usecase.profile.StationProfileOutcome
import com.anpfuel.domain.profile.StationProfile
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

data class StationProfileState(
    val profile: StationProfile? = null,
    val loading: Boolean = false,
    val unavailable: Boolean = false,
    val stale: Boolean = false,
)

@HiltViewModel
class StationProfileViewModel @Inject constructor(
    savedStateHandle: SavedStateHandle,
    private val read: GetStationProfileUseCase,
) : ViewModel() {
    val stationId: String = checkNotNull(savedStateHandle["stationId"])
    private val mutableState = MutableStateFlow(StationProfileState())
    val state = mutableState.asStateFlow()

    init { refresh() }

    fun refresh() {
        if (mutableState.value.loading) return
        mutableState.value = mutableState.value.copy(loading = true)
        viewModelScope.launch {
            try {
                val found = (read(stationId) as? StationProfileOutcome.Found)?.profile
                mutableState.value = StationProfileState(profile = found, unavailable = found == null)
            } catch (error: CancellationException) {
                throw error
            } catch (_: Exception) {
                mutableState.value = mutableState.value.copy(
                    loading = false, unavailable = true, stale = mutableState.value.profile != null,
                )
            }
        }
    }
}
