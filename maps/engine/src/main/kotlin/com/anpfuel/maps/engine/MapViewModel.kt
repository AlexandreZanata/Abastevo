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

/** Native overlay station names render from this street zoom up; below it only pins show. */
internal const val OVERLAY_LABELS_MIN_ZOOM = 15

data class MapUiState(
    val ready: Boolean = false,
    val hasLocationPermission: Boolean = false,
    val approximateLocation: Boolean = false,
    val deniedForever: Boolean = false,
    val gpsEnabled: Boolean = true,
    val recenterError: String? = null,
    val initError: String? = null,
    val camera: MapCamera? = null,
)

@HiltViewModel
class MapViewModel @Inject constructor(
    private val engine: MapEngine,
    private val cameraStore: MapCameraStore,
) : ViewModel() {
    private var defaultCamera: MapCamera = MapCamera(-25.42, -49.27, 13)
    private var disposed = false
    private var initializing = false
    /** Native bookmark labels render above this zoom only; the SDK default is shown. */
    private var overlayLabelsVisible = true
    private val _state = MutableStateFlow(MapUiState())
    val state: StateFlow<MapUiState> = _state.asStateFlow()

    /**
     * Single owner of native initialization. Safe to call on every
     * composition: the first call starts the engine once on the application
     * context (never a retained Activity), later calls are no-ops, and a
     * synchronous failure surfaces [MapUiState.initError] with retry.
     */
    fun ensureInitialized(context: Context) {
        if (disposed || _state.value.ready || initializing) return
        initializing = true
        _state.update { it.copy(initError = null) }
        val started = try {
            engine.initialize(context.applicationContext) { onEngineReady() }
        } catch (_: Exception) {
            false
        }
        if (!started) {
            initializing = false
            _state.update { it.copy(initError = "map-init-failed") }
        }
    }

    fun retryInit(context: Context) {
        if (disposed || _state.value.ready) return
        initializing = false
        ensureInitialized(context)
    }

    /** Test-only secondary constructor preserves the frozen default camera. */
    constructor(
        engine: MapEngine,
        cameraStore: MapCameraStore,
        defaultCamera: MapCamera,
    ) : this(engine, cameraStore) {
        this.defaultCamera = defaultCamera
    }

    fun newMapView(context: Context): View = engine.newMapView(context)

    fun isRenderingReady(): Boolean = _state.value.ready && engine.isRenderingReady()

    fun onEngineReady() {
        if (disposed || _state.value.ready) return
        initializing = false
        val camera = cameraStore.load() ?: defaultCamera
        _state.update { it.copy(ready = true, camera = camera, recenterError = null, initError = null) }
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
            val centered = engine.currentCenter()
            _state.update {
                val base = it.camera ?: defaultCamera
                it.copy(
                    recenterError = null,
                    camera = base.copy(
                        latitude = centered?.latitude ?: base.latitude,
                        longitude = centered?.longitude ?: base.longitude,
                        zoom = 15,
                    ),
                )
            }
        } else {
            // First fix not yet available; browsing stays usable.
            _state.update { it.copy(recenterError = "no-fix") }
        }
    }

    fun setViewport(latitude: Double, longitude: Double, zoom: Int = 13) {
        if (disposed) return
        val next = MapCamera(latitude, longitude, zoom.coerceIn(1, 20))
        cameraStore.save(next)
        _state.update { it.copy(camera = next) }
        if (_state.value.ready) {
            engine.setViewport(next.latitude, next.longitude, next.zoom)
        }
    }

    fun syncCameraFromEngine(): MapCamera? {
        if (disposed || !_state.value.ready) return null
        val current = _state.value.camera ?: defaultCamera
        val native = engine.currentCamera() ?: engine.currentCenter()?.let {
            current.copy(latitude = it.latitude, longitude = it.longitude)
        } ?: return _state.value.camera
        applyOverlayLabelsVisibility(native.zoom)
        val dLat = kotlin.math.abs(native.latitude - current.latitude)
        val dLon = kotlin.math.abs(native.longitude - current.longitude)
        if (dLat >= 0.0005 || dLon >= 0.0005 || native.zoom != current.zoom) {
            val updated = native.copy(zoom = native.zoom.coerceIn(1, 20))
            _state.update { it.copy(camera = updated) }
            return updated
        }
        return current
    }

    fun radiusForZoom(zoom: Int): Int = when {
        zoom >= 16 -> 2_000
        zoom == 15 -> 4_000
        zoom == 14 -> 7_000
        zoom == 13 -> 10_000
        else -> 15_000
    }

    fun onHostStop() {
        if (disposed) return
        val camera = syncCameraFromEngine() ?: return
        cameraStore.save(camera)
        _state.update { it.copy(camera = camera) }
    }

    /**
     * Station-name clutter: native bookmark labels render from street zoom up.
     * Per-pin text color, size, halo and min-zoom are owned by the pinned map
     * style, so visibility is the only supported lever; pins stay tappable.
     */
    private fun applyOverlayLabelsVisibility(zoom: Int) {
        val visible = zoom >= OVERLAY_LABELS_MIN_ZOOM
        if (visible != overlayLabelsVisible) {
            overlayLabelsVisible = visible
            engine.setOverlayLabelsVisible(visible)
        }
    }

    fun dispose() {
        if (disposed) return
        disposed = true
        // Navigation owns its session in the foreground service; leaving a tab or
        // recreating an Activity must not destroy guidance, preview cancellation
        // must not stop it, and only explicit stop releases native following.
        if (engine.isGuiding()) {
            engine.cancelSearch()
            engine.cancelRoute()
            return
        }
        engine.release()
    }

    override fun onCleared() {
        dispose()
        super.onCleared()
    }
}
