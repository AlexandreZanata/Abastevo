package com.anpfuel.app.ui.map

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.mapsaved.GetSavedTripUseCase
import com.anpfuel.domain.map.saved.SavedTrip
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** Loads one owner-scoped saved route plan for reopening on the map. */
@HiltViewModel
class MapPlanViewModel @Inject constructor(
    private val getTrip: GetSavedTripUseCase,
) : ViewModel() {
    private val _trip = MutableStateFlow<SavedTrip?>(null)
    val trip = _trip.asStateFlow()

    fun load(id: String) {
        if (id.isBlank()) return
        viewModelScope.launch { _trip.value = getTrip(id) }
    }
}
