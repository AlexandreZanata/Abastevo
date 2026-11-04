package com.anpfuel.app.ui.map

import android.Manifest
import android.annotation.SuppressLint
import android.content.Context
import android.content.pm.PackageManager
import android.location.Location
import android.location.LocationManager
import android.os.Looper
import android.os.SystemClock
import androidx.core.content.ContextCompat
import androidx.core.location.LocationListenerCompat
import androidx.core.location.LocationManagerCompat
import androidx.core.location.LocationRequestCompat
import com.anpfuel.domain.routing.RoutePoint
import kotlin.coroutines.resume
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withTimeoutOrNull

private const val ROUTE_FIX_TIMEOUT_MS = 15_000L
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
    return withTimeoutOrNull(ROUTE_FIX_TIMEOUT_MS) {
        awaitRouteOrigin(manager, activeProviders, label)
    }
}

@SuppressLint("MissingPermission")
private suspend fun awaitRouteOrigin(
    manager: LocationManager,
    providers: List<String>,
    label: String,
): RoutePoint? = suspendCancellableCoroutine { continuation ->
    val listener = object : LocationListenerCompat {
        override fun onLocationChanged(location: Location) {
            if (!continuation.isActive) return
            val origin = location.toRouteOrigin(label, SystemClock.elapsedRealtimeNanos()) ?: return
            LocationManagerCompat.removeUpdates(manager, this)
            continuation.resume(origin)
        }
    }
    val request = LocationRequestCompat.Builder(1_000L)
        .setQuality(LocationRequestCompat.QUALITY_HIGH_ACCURACY)
        .setDurationMillis(ROUTE_FIX_TIMEOUT_MS)
        .build()
    var registered = false
    for (provider in providers) {
        try {
            LocationManagerCompat.requestLocationUpdates(manager, provider, request, listener, Looper.getMainLooper())
            registered = true
        } catch (_: SecurityException) {
            // A grant may have changed since the caller check.
        } catch (_: IllegalArgumentException) {
            // The provider may have disappeared since enumeration.
        }
    }
    if (registered) {
        continuation.invokeOnCancellation { LocationManagerCompat.removeUpdates(manager, listener) }
    } else {
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
