package com.anpfuel.app.ui.map

import android.Manifest
import android.annotation.SuppressLint
import android.content.Context
import android.content.pm.PackageManager
import android.location.Location
import android.location.LocationManager
import android.os.SystemClock
import androidx.core.content.ContextCompat
import androidx.core.location.LocationManagerCompat
import androidx.core.os.CancellationSignal
import com.anpfuel.domain.routing.RoutePoint
import java.util.concurrent.Executor
import kotlin.coroutines.resume
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.channelFlow
import kotlinx.coroutines.flow.firstOrNull
import kotlinx.coroutines.launch
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withTimeoutOrNull

private const val ROUTE_FIX_TIMEOUT_MS = 15_000L
private const val ROUTE_FIX_RETRY_MS = 1_000L
private const val ROUTE_FIX_MAX_AGE_NS = 15_000_000_000L

@SuppressLint("MissingPermission")
internal suspend fun findCurrentRoutePoint(
    context: Context,
    label: String,
    mapLocation: () -> Location?,
): RoutePoint? {
    val fine = ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) ==
        PackageManager.PERMISSION_GRANTED
    val coarse = ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_COARSE_LOCATION) ==
        PackageManager.PERMISSION_GRANTED
    if (!fine && !coarse) return null
    val manager = context.getSystemService(LocationManager::class.java) ?: return null
    if (!LocationManagerCompat.isLocationEnabled(manager)) return null

    // A map snapshot can contain precise coordinates from an earlier grant.
    if (fine) mapLocation()?.toRouteOrigin(label, SystemClock.elapsedRealtimeNanos())?.let { return it }

    val providers = manager.getProviders(true).filter { fine || it != LocationManager.GPS_PROVIDER }
    val cached = providers.mapNotNull { provider ->
        try {
            manager.getLastKnownLocation(provider)
        } catch (_: SecurityException) {
            null
        } catch (_: IllegalArgumentException) {
            null
        }
    }
    val now = SystemClock.elapsedRealtimeNanos()
    cached.sortedByDescending { it.elapsedRealtimeNanos }
        .firstNotNullOfOrNull { it.toRouteOrigin(label, now) }
        ?.let { return it }

    val activeProviders = providers.filterNot { it == LocationManager.PASSIVE_PROVIDER }
    if (activeProviders.isEmpty()) return null
    val executor = ContextCompat.getMainExecutor(context)
    return withTimeoutOrNull(ROUTE_FIX_TIMEOUT_MS) {
        channelFlow {
            for (provider in activeProviders) {
                launch {
                    awaitUsableRouteOrigin(manager, provider, executor, label)?.let { send(it) }
                }
            }
        }.firstOrNull()
    }
}

private suspend fun awaitUsableRouteOrigin(
    manager: LocationManager,
    provider: String,
    executor: Executor,
    label: String,
): RoutePoint? {
    while (true) {
        val location = awaitCurrentLocation(manager, provider, executor) ?: return null
        location.toRouteOrigin(label, SystemClock.elapsedRealtimeNanos())?.let { return it }
        // A one-shot result can be a recent but unsuitable cached fix.
        delay(ROUTE_FIX_RETRY_MS)
    }
}

@SuppressLint("MissingPermission")
@Suppress("DEPRECATION")
private suspend fun awaitCurrentLocation(
    manager: LocationManager,
    provider: String,
    executor: Executor,
): Location? = suspendCancellableCoroutine { continuation ->
    val cancellation = CancellationSignal()
    try {
        LocationManagerCompat.getCurrentLocation(manager, provider, cancellation, executor) { location ->
            continuation.resume(location)
        }
        continuation.invokeOnCancellation { cancellation.cancel() }
    } catch (_: SecurityException) {
        continuation.resume(null)
    } catch (_: IllegalArgumentException) {
        continuation.resume(null)
    }
}

internal fun Location.toRouteOrigin(label: String, nowNanos: Long): RoutePoint? {
    val sampledAt = elapsedRealtimeNanos
    if (sampledAt <= 0 || nowNanos - sampledAt !in 0..ROUTE_FIX_MAX_AGE_NS) return null
    if (!hasAccuracy() || accuracy !in 0f..100f) return null
    if (latitude !in -90.0..90.0 || longitude !in -180.0..180.0) return null
    return RoutePoint(latitude, longitude, label)
}
