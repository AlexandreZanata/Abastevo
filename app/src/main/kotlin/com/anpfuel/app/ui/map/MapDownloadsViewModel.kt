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

data class MapDownloadsUi(
    val loading: Boolean = true,
    val activeGeneration: String? = null,
    val packageCount: Int = 0,
)

@HiltViewModel
class MapDownloadsViewModel @Inject constructor(
    private val store: MapPackageStore,
) : ViewModel() {
    private val _ui = MutableStateFlow(MapDownloadsUi())
    val ui: StateFlow<MapDownloadsUi> = _ui.asStateFlow()

    init {
        refresh()
    }

    fun refresh() {
        viewModelScope.launch {
            _ui.update { it.copy(loading = true) }
            try {
                val active = store.activeGeneration()
                val count = if (active == null) 0 else store.packages(active.generation).size
                _ui.update { it.copy(loading = false, activeGeneration = active?.generation, packageCount = count) }
            } catch (_: Exception) {
                _ui.update { it.copy(loading = false, activeGeneration = null, packageCount = 0) }
            }
        }
    }
}
