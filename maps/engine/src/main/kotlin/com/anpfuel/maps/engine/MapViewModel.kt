package com.anpfuel.maps.engine

import android.content.Context
import android.view.View
import androidx.lifecycle.ViewModel
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

data class MapUiState(
    val ready: Boolean = false,
    val hasLocationPermission: Boolean = false,
    val approximateLocation: Boolean = false,
    val deniedForever: Boolean = false,
    val gpsEnabled: Boolean = true,
    val recenterError: String? = null,
    val camera: MapCamera? = null,
)

@HiltViewModel
class MapViewModel @Inject constructor(
    private val engine: MapEngine,
    private val cameraStore: MapCameraStore,
) : ViewModel() {
    private var defaultCamera: MapCamera = MapCamera(-25.42, -49.27, 13)
    private var disposed = false
    private val _state = MutableStateFlow(MapUiState())
    val state: StateFlow<MapUiState> = _state.asStateFlow()

    /** Test-only secondary constructor preserves the frozen default camera. */
    constructor(
        engine: MapEngine,
        cameraStore: MapCameraStore,
        defaultCamera: MapCamera,
    ) : this(engine, cameraStore) {
        this.defaultCamera = defaultCamera
    }

    fun newMapView(context: Context): View = engine.newMapView(context)

    fun onEngineReady() {
        if (disposed || _state.value.ready) return
        val camera = cameraStore.load() ?: defaultCamera
        _state.update { it.copy(ready = true, camera = camera, recenterError = null) }
        engine.setViewport(camera.latitude, camera.longitude, camera.zoom)
    }

    fun onPermissionState(fine: Boolean, coarse: Boolean, deniedForever: Boolean) {
        if (disposed) return
        _state.update {
            it.copy(
                hasLocationPermission = fine || coarse,
                approximateLocation = !fine && coarse,
                deniedForever = deniedForever,
                recenterError = null,
            )
        }
    }

    fun onGpsState(enabled: Boolean) {
        if (disposed) return
        _state.update { it.copy(gpsEnabled = enabled, recenterError = null) }
    }

    fun onRecenter() {
        if (disposed) return
        if (!_state.value.hasLocationPermission) {
            _state.update { it.copy(recenterError = "no-permission") }
            return
        }
        if (!_state.value.gpsEnabled) {
            _state.update { it.copy(recenterError = "gps-off") }
            return
        }
        if (engine.recenter()) {
            _state.update {
                it.copy(
                    recenterError = null,
                    camera = it.camera?.copy(zoom = 15) ?: defaultCamera.copy(zoom = 15),
                )
            }
        } else {
            // First fix not yet available; browsing stays usable.
            _state.update { it.copy(recenterError = "no-fix") }
        }
    }

    fun onHostStop() {
        if (disposed) return
        val camera = _state.value.camera ?: return
        engine.currentCenter()?.let {
            val saved = camera.copy(latitude = it.latitude, longitude = it.longitude)
            cameraStore.save(saved)
            _state.update { state -> state.copy(camera = saved) }
        }
    }

    fun dispose() {
        if (disposed) return
        disposed = true
        engine.release()
    }

    override fun onCleared() {
        dispose()
        super.onCleared()
    }
}
