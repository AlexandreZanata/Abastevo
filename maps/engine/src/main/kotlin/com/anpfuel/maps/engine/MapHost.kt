package com.anpfuel.maps.engine

import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.Spacer
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Arrangement
import android.Manifest
import android.app.Activity
import android.content.Context
import android.content.pm.PackageManager
import android.location.LocationManager
import android.view.View
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.ContextCompat
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel

/**
 * Product host: singleton engine + Hilt view model. Browsing never requires
 * GPS and never starts continuous location; permission is requested only on
 * explicit recenter tap.
 */
@Composable
fun MapHost(
    modifier: Modifier = Modifier,
    recenterDescription: String = "Recenter map",
    initErrorMessage: String = "Map unavailable",
    retryLabel: String = "Retry",
    locationDeniedMessage: String = "Location denied. Browsing stays available.",
    gpsOffMessage: String = "GPS off. Browsing stays available.",
    waitingForFixMessage: String = "Waiting for first fix.",
    onViewportChanged: ((latitude: Double, longitude: Double, radiusMeters: Int) -> Unit)? = null,
    onReady: ((Boolean) -> Unit)? = null,
    active: Boolean = true,
    recenterBottomInset: Dp = 16.dp,
    /** Follows the app theme; null leaves the native appearance untouched. */
    darkMode: Boolean? = null,
    /** App-owned recenter glyph (the engine module ships no icon set). */
    recenterIcon: @Composable () -> Unit = { Text("◎", style = MaterialTheme.typography.titleLarge) },
    /** Extra round map controls stacked above recenter (for example layers/filters). */
    extraControls: @Composable ColumnScope.() -> Unit = {},
    /** Height of app-owned controls drawn just above recenter; status pills clear them. */
    statusClearance: Dp = 0.dp,
    showRecenterControls: Boolean = true,
) {
    val vm: MapViewModel = hiltViewModel()
    MapHostContent(
        vm = vm,
        modifier = modifier,
        recenterDescription = recenterDescription,
        initErrorMessage = initErrorMessage,
        retryLabel = retryLabel,
        locationDeniedMessage = locationDeniedMessage,
        gpsOffMessage = gpsOffMessage,
        waitingForFixMessage = waitingForFixMessage,
        onViewportChanged = onViewportChanged,
        onReady = onReady,
        active = active,
        recenterBottomInset = recenterBottomInset,
        darkMode = darkMode,
        recenterIcon = recenterIcon,
        extraControls = extraControls,
        statusClearance = statusClearance,
        showRecenterControls = showRecenterControls,
    )
}

/**
 * Testable host with an explicit engine (fake or real). Production uses the
 * Hilt overload above; tests inject a fake to control readiness/permission.
 */
@Composable
fun MapHost(
    engine: MapEngine,
    modifier: Modifier = Modifier,
    cameraStore: MapCameraStore? = null,
    recenterDescription: String = "Recenter map",
    initErrorMessage: String = "Map unavailable",
    retryLabel: String = "Retry",
    locationDeniedMessage: String = "Location denied. Browsing stays available.",
    gpsOffMessage: String = "GPS off. Browsing stays available.",
    waitingForFixMessage: String = "Waiting for first fix.",
    onViewportChanged: ((latitude: Double, longitude: Double, radiusMeters: Int) -> Unit)? = null,
    onReady: ((Boolean) -> Unit)? = null,
    active: Boolean = true,
    recenterBottomInset: Dp = 16.dp,
    /** Follows the app theme; null leaves the native appearance untouched. */
    darkMode: Boolean? = null,
    /** App-owned recenter glyph (the engine module ships no icon set). */
    recenterIcon: @Composable () -> Unit = { Text("◎", style = MaterialTheme.typography.titleLarge) },
    /** Extra round map controls stacked above recenter (for example layers/filters). */
    extraControls: @Composable ColumnScope.() -> Unit = {},
    /** Height of app-owned controls drawn just above recenter; status pills clear them. */
    statusClearance: Dp = 0.dp,
    showRecenterControls: Boolean = true,
) {
    val context = LocalContext.current
    val store = cameraStore ?: remember(context) { SharedPreferencesMapCameraStore(context) }
    val vm: MapViewModel = viewModel(
        key = "map-${engine.hashCode()}",
        factory = remember(engine, store) {
            object : ViewModelProvider.Factory {
                override fun <T : ViewModel> create(modelClass: Class<T>): T {
                    @Suppress("UNCHECKED_CAST")
                    return MapViewModel(engine, store) as T
                }
            }
        },
    )
    MapHostContent(
        vm = vm,
        modifier = modifier,
        recenterDescription = recenterDescription,
        initErrorMessage = initErrorMessage,
        retryLabel = retryLabel,
        locationDeniedMessage = locationDeniedMessage,
        gpsOffMessage = gpsOffMessage,
        waitingForFixMessage = waitingForFixMessage,
        onViewportChanged = onViewportChanged,
        onReady = onReady,
        active = active,
        recenterBottomInset = recenterBottomInset,
        darkMode = darkMode,
        recenterIcon = recenterIcon,
        extraControls = extraControls,
        statusClearance = statusClearance,
        showRecenterControls = showRecenterControls,
    )
}

@Composable
private fun MapHostContent(
    vm: MapViewModel,
    modifier: Modifier = Modifier,
    recenterDescription: String = "Recenter map",
    initErrorMessage: String = "Map unavailable",
    retryLabel: String = "Retry",
    locationDeniedMessage: String = "Location denied. Browsing stays available.",
    gpsOffMessage: String = "GPS off. Browsing stays available.",
    waitingForFixMessage: String = "Waiting for first fix.",
    onViewportChanged: ((latitude: Double, longitude: Double, radiusMeters: Int) -> Unit)? = null,
    onReady: ((Boolean) -> Unit)? = null,
    active: Boolean = true,
    recenterBottomInset: Dp = 16.dp,
    /** Follows the app theme; null leaves the native appearance untouched. */
    darkMode: Boolean? = null,
    /** App-owned recenter glyph (the engine module ships no icon set). */
    recenterIcon: @Composable () -> Unit = { Text("◎", style = MaterialTheme.typography.titleLarge) },
    /** Extra round map controls stacked above recenter (for example layers/filters). */
    extraControls: @Composable ColumnScope.() -> Unit = {},
    /** Height of app-owned controls drawn just above recenter; status pills clear them. */
    statusClearance: Dp = 0.dp,
    showRecenterControls: Boolean = true,
) {
    val context = LocalContext.current
    val lifecycleOwner = LocalLifecycleOwner.current
    val state by vm.state.collectAsStateWithLifecycle()
    LaunchedEffect(darkMode) {
        if (darkMode != null) {
            app.organicmaps.sdk.MapAppearance.set(
                if (darkMode) app.organicmaps.sdk.MapAppearance.Dark
                else app.organicmaps.sdk.MapAppearance.Light,
            )
        }
    }
    val permissionLauncher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        val fine = ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED
        val coarse = ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_COARSE_LOCATION) == PackageManager.PERMISSION_GRANTED
        val activity = context as? Activity
        val deniedForever = !granted && !fine && !coarse &&
            (activity == null || !activity.shouldShowRequestPermissionRationale(Manifest.permission.ACCESS_FINE_LOCATION))
        vm.onPermissionState(fine, coarse, deniedForever)
        if (granted) vm.onRecenter()
    }

    LaunchedEffect(vm) {
        updatePermission(context, vm)
        vm.onGpsState(isGpsEnabled(context))
        vm.ensureInitialized(context.applicationContext)
    }
    LaunchedEffect(state.ready) {
        if (!state.ready) {
            onReady?.invoke(false)
            return@LaunchedEffect
        }
        var rendered = vm.isRenderingReady()
        while (!rendered) {
            kotlinx.coroutines.delay(120)
            rendered = vm.isRenderingReady()
        }
        onReady?.invoke(true)
    }
    LaunchedEffect(state.ready, state.camera, active) {
        val cam = state.camera
        if (active && state.ready && cam != null) {
            onViewportChanged?.invoke(cam.latitude, cam.longitude, vm.radiusForZoom(cam.zoom))
        }
    }
    LaunchedEffect(vm, state.ready, onViewportChanged != null, active) {
        if (active && state.ready && onViewportChanged != null) {
            while (true) {
                kotlinx.coroutines.delay(600)
                vm.syncCameraFromEngine()
            }
        }
    }
    DisposableEffect(lifecycleOwner) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) {
                updatePermission(context, vm)
                vm.onGpsState(isGpsEnabled(context))
            }
            if (event == Lifecycle.Event.ON_STOP) vm.onHostStop()
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
    }
    Box(modifier.fillMaxSize()) {
        if (state.initError != null && !state.ready) {
            androidx.compose.foundation.layout.Column(
                modifier = Modifier.align(Alignment.Center),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                Text(
                    initErrorMessage,
                    style = MaterialTheme.typography.titleMedium,
                )
                androidx.compose.material3.TextButton(
                    onClick = { vm.retryInit(context.applicationContext) },
                    modifier = Modifier.size(width = 160.dp, height = 48.dp).semantics {
                        contentDescription = retryLabel
                    },
                ) {
                    Text(retryLabel)
                }
            }
        }
        if (state.ready) {
            AndroidView(
                factory = { ctx ->
                    vm.newMapView(ctx).also { view ->
                        view.visibility = if (active) View.VISIBLE else View.INVISIBLE
                    }
                },
                update = { view ->
                    view.visibility = if (active) View.VISIBLE else View.INVISIBLE
                },
                modifier = Modifier.fillMaxSize(),
            )
        }
        if (state.ready && showRecenterControls) {
            Column(
                modifier = Modifier.align(Alignment.BottomEnd).padding(
                    start = 16.dp,
                    top = 16.dp,
                    end = 16.dp,
                    bottom = recenterBottomInset,
                ),
                horizontalAlignment = Alignment.End,
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                extraControls()
                // Permission/GPS states never block browsing; a pill beside the
                // recenter control annotates it without covering other controls.
                val status: Pair<String, Boolean>? = when {
                    !state.hasLocationPermission && state.deniedForever -> locationDeniedMessage to false
                    !state.gpsEnabled -> gpsOffMessage to false
                    state.recenterError == "no-fix" -> waitingForFixMessage to true
                    else -> null
                }
                if (status != null) {
                    Surface(
                        shape = RoundedCornerShape(20.dp),
                        color = MaterialTheme.colorScheme.inverseSurface,
                        contentColor = MaterialTheme.colorScheme.inverseOnSurface,
                        shadowElevation = 4.dp,
                        modifier = Modifier.widthIn(max = 300.dp).semantics { liveRegion = LiveRegionMode.Polite },
                    ) {
                        Row(
                            modifier = Modifier.padding(horizontal = 14.dp, vertical = 10.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(10.dp),
                        ) {
                            if (status.second) {
                                CircularProgressIndicator(
                                    modifier = Modifier.size(16.dp),
                                    strokeWidth = 2.dp,
                                    color = MaterialTheme.colorScheme.inversePrimary,
                                )
                            } else {
                                Text("!", style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.inversePrimary)
                            }
                            Text(status.first, style = MaterialTheme.typography.labelLarge, maxLines = 2, overflow = TextOverflow.Ellipsis)
                        }
                    }
                    // Keeps the pill above controls the app draws over this column.
                    if (statusClearance > 0.dp) Spacer(Modifier.height(statusClearance))
                }
                Surface(
                    shape = CircleShape,
                    color = MaterialTheme.colorScheme.surfaceContainerHigh,
                    contentColor = MaterialTheme.colorScheme.primary,
                    shadowElevation = 6.dp,
                ) {
                    IconButton(
                        onClick = {
                            if (state.hasLocationPermission) {
                                vm.onRecenter()
                            } else {
                                permissionLauncher.launch(Manifest.permission.ACCESS_FINE_LOCATION)
                            }
                        },
                        modifier = Modifier.size(56.dp).semantics {
                            contentDescription = recenterDescription
                        },
                    ) {
                        recenterIcon()
                    }
                }
            }
        }
    }
}

private fun updatePermission(context: Context, vm: MapViewModel) {
    val fine = ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED
    val coarse = ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_COARSE_LOCATION) == PackageManager.PERMISSION_GRANTED
    vm.onPermissionState(fine, coarse, deniedForever = false)
}

private fun isGpsEnabled(context: Context): Boolean {
    val manager = context.getSystemService(Context.LOCATION_SERVICE) as? LocationManager
        ?: return true
    return try {
        manager.isProviderEnabled(LocationManager.GPS_PROVIDER) ||
            manager.isProviderEnabled(LocationManager.NETWORK_PROVIDER)
    } catch (_: Exception) {
        true
    }
}
