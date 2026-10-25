package com.anpfuel.app.ui.map

import androidx.lifecycle.ViewModel
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

enum class MapPriceSource { ANP, COMMUNITY }

data class MapScreenUi(
    val source: MapPriceSource = MapPriceSource.ANP,
    val loading: Boolean = false,
    val error: String? = null,
    val noPackage: Boolean = true,
)

/**
 * Minimal MAP-17 screen state. Source switching is explicit and discards
 * older responses by generation (full invalidation lives in MAP-18/19;
 * this shell owns the selector identity).
 */
@HiltViewModel
class MapScreenViewModel @Inject constructor() : ViewModel() {
    private val _ui = MutableStateFlow(MapScreenUi())
    val ui: StateFlow<MapScreenUi> = _ui.asStateFlow()

    fun selectSource(source: MapPriceSource) {
        _ui.update { it.copy(source = source, error = null) }
    }

    fun retry() {
        _ui.update { it.copy(loading = false, error = null) }
    }

    fun setNoPackage(hasPackage: Boolean) {
        _ui.update { it.copy(noPackage = !hasPackage) }
    }
}
