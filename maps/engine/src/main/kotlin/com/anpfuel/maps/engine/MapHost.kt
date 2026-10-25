package com.anpfuel.maps.engine

import android.Manifest
import android.app.Activity
import android.content.Context
import android.content.pm.PackageManager
import android.location.LocationManager
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
) {
    val vm: MapViewModel = hiltViewModel()
    MapHostContent(vm = vm, engine = null, modifier = modifier)
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
    MapHostContent(vm = vm, engine = engine, modifier = modifier)
}

@Composable
private fun MapHostContent(
    vm: MapViewModel,
    engine: MapEngine?,
    modifier: Modifier = Modifier,
) {
    val context = LocalContext.current
    val lifecycleOwner = LocalLifecycleOwner.current
    val state by vm.state.collectAsStateWithLifecycle()
    val permissionLauncher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        val fine = ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED
        val coarse = ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_COARSE_LOCATION) == PackageManager.PERMISSION_GRANTED
        val activity = context as? Activity
        val deniedForever = !granted && !fine && !coarse &&
            (activity == null || !activity.shouldShowRequestPermissionRationale(Manifest.permission.ACCESS_FINE_LOCATION))
        vm.onPermissionState(fine, coarse, deniedForever)
        if (granted) vm.onRecenter()
    }

    LaunchedEffect(engine) {
        updatePermission(context, vm)
        vm.onGpsState(isGpsEnabled(context))
        engine?.initialize(context) { vm.onEngineReady() }
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
        if (state.ready) {
            AndroidView(
                factory = { vm.newMapView(it) },
                modifier = Modifier.fillMaxSize(),
            )
        }
        if (state.ready) {
            Column(modifier = Modifier.align(Alignment.BottomEnd).padding(16.dp)) {
                // Permission/GPS states never block browsing; they annotate recenter.
                if (!state.hasLocationPermission && state.deniedForever) {
                    Surface(shape = MaterialTheme.shapes.small, tonalElevation = 3.dp) {
                        Text(
                            "Location denied. Browsing stays available.",
                            modifier = Modifier.padding(8.dp),
                            style = MaterialTheme.typography.bodySmall,
                        )
                    }
                } else if (!state.gpsEnabled) {
                    Surface(shape = MaterialTheme.shapes.small, tonalElevation = 3.dp) {
                        Text(
                            "GPS off. Browsing stays available.",
                            modifier = Modifier.padding(8.dp),
                            style = MaterialTheme.typography.bodySmall,
                        )
                    }
                } else if (state.recenterError == "no-fix") {
                    Surface(shape = MaterialTheme.shapes.small, tonalElevation = 3.dp) {
                        Text(
                            "Waiting for first fix.",
                            modifier = Modifier.padding(8.dp),
                            style = MaterialTheme.typography.bodySmall,
                        )
                    }
                }
                Surface(
                    shape = MaterialTheme.shapes.small,
                    tonalElevation = 3.dp,
                ) {
                    IconButton(
                        onClick = {
                            if (state.hasLocationPermission) {
                                vm.onRecenter()
                            } else {
                                permissionLauncher.launch(Manifest.permission.ACCESS_FINE_LOCATION)
                            }
                        },
                        modifier = Modifier.size(48.dp).semantics {
                            contentDescription = "Recenter map"
                        },
                    ) {
                        Text("◎", style = MaterialTheme.typography.titleLarge)
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
