package com.anpfuel.maps.engine

import android.content.Context
import android.view.View
import androidx.lifecycle.ViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

data class MapUiState(
    val ready: Boolean = false,
    val hasLocationPermission: Boolean = false,
    val approximateLocation: Boolean = false,
    val deniedForever: Boolean = false,
    val camera: MapCamera? = null,
)

class MapViewModel(
    private val engine: MapEngine,
    private val cameraStore: MapCameraStore,
    private val defaultCamera: MapCamera = MapCamera(-25.42, -49.27, 13),
) : ViewModel() {
    private val _state = MutableStateFlow(MapUiState())
    val state: StateFlow<MapUiState> = _state.asStateFlow()

    fun newMapView(context: Context): View = engine.newMapView(context)

    fun onEngineReady() {
        if (_state.value.ready) return
        val camera = cameraStore.load() ?: defaultCamera
        _state.update { it.copy(ready = true, camera = camera) }
        engine.setViewport(camera.latitude, camera.longitude, camera.zoom)
    }

    fun onPermissionState(fine: Boolean, coarse: Boolean, deniedForever: Boolean) {
        _state.update {
            it.copy(
                hasLocationPermission = fine || coarse,
                approximateLocation = !fine && coarse,
                deniedForever = deniedForever,
            )
        }
    }

    fun onRecenter() {
        if (!_state.value.hasLocationPermission) return
        if (engine.recenter()) {
            _state.update {
                it.copy(camera = it.camera?.copy(zoom = 15) ?: MapCamera(-25.42, -49.27, 15))
            }
        }
    }

    fun onHostStop() {
        val camera = _state.value.camera ?: return
        engine.currentCenter()?.let {
            val saved = camera.copy(latitude = it.latitude, longitude = it.longitude)
            cameraStore.save(saved)
            _state.update { state -> state.copy(camera = saved) }
        }
    }

    fun dispose() {
        engine.cancelSearch()
        engine.cancelRoute()
    }

    override fun onCleared() {
        dispose()
        super.onCleared()
    }
}
