package com.anpfuel.app.ui.map

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.data.map.MapPackageStore
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

enum class MapPriceSource { ANP, COMMUNITY }

data class MapScreenUi(
    val source: MapPriceSource = MapPriceSource.ANP,
    val loading: Boolean = false,
    val error: String? = null,
    val noPackage: Boolean = true,
)

/**
 * MAP-17 shell state plus MAP-15 availability: the banner shows only when no
 * generation is active in Room. Source switching stays explicit.
 */
@HiltViewModel
class MapScreenViewModel @Inject constructor(
    private val store: MapPackageStore,
) : ViewModel() {
    private val _ui = MutableStateFlow(MapScreenUi())
    val ui: StateFlow<MapScreenUi> = _ui.asStateFlow()

    init {
        refreshAvailability()
    }

    fun selectSource(source: MapPriceSource) {
        _ui.update { it.copy(source = source, error = null) }
    }

    fun retry() {
        _ui.update { it.copy(loading = true, error = null) }
        refreshAvailability()
    }

    fun refreshAvailability() {
        viewModelScope.launch {
            try {
                val active = store.activeGeneration()
                _ui.update { it.copy(loading = false, noPackage = active == null) }
            } catch (_: Exception) {
                _ui.update { it.copy(loading = false, error = "availability-failed") }
            }
        }
    }
}
