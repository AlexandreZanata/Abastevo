package com.anpfuel.app.location

import android.Manifest
import android.annotation.SuppressLint
import android.content.Context
import android.content.pm.PackageManager
import android.location.Location
import android.location.LocationListener
import android.location.LocationManager
import android.os.Looper
import androidx.core.content.ContextCompat
import com.anpfuel.domain.valueobject.DeviceLocation
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton
import kotlin.coroutines.resume
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withTimeoutOrNull

/**
 * One-shot device location access for UC-012 onboarding and the UC-015 nearest-station
 * search (coordinates are never persisted, BR-021).
 */
@Singleton
class LocationPermissionHandler @Inject constructor(
    @ApplicationContext private val context: Context,
) {

    fun hasLocationPermission(): Boolean =
        ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) ==
            PackageManager.PERMISSION_GRANTED ||
            ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_COARSE_LOCATION) ==
            PackageManager.PERMISSION_GRANTED

    /**
     * Cached device location from any provider, without requesting a fresh fix.
     * The most recent fix across providers wins; returns null when none exists.
     */
    @SuppressLint("MissingPermission")
    fun getLastKnownLocation(): DeviceLocation? {
        if (!hasLocationPermission()) {
            return null
        }

        val locationManager = context.getSystemService(LocationManager::class.java)
        val location = lastKnownLocationCandidates(locationManager)
            .maxByOrNull { it.time }
            ?: return null

        return DeviceLocation.of(location.latitude, location.longitude)
    }

    /**
     * UC-015 — freshest available device location.
     *
     * Reuses the most recent cached fix across providers when it is fresh enough, otherwise
     * requests a single fresh fix with a bounded timeout. Returns null when no fix can be
     * obtained within [timeoutMillis] (for example, GPS with no sky view and no network
     * location), never blocking indefinitely.
     */
    @SuppressLint("MissingPermission")
    suspend fun getCurrentLocation(timeoutMillis: Long = DEFAULT_FIX_TIMEOUT_MILLIS): DeviceLocation? {
        if (!hasLocationPermission()) {
            return null
        }

        val locationManager = context.getSystemService(LocationManager::class.java)
        lastKnownLocationCandidates(locationManager)
            .maxByOrNull { it.time }
            ?.takeIf { isFresh(it) }
            ?.let { return DeviceLocation.of(it.latitude, it.longitude) }

        val location = withTimeoutOrNull(timeoutMillis) {
            awaitSingleFix(locationManager)
        } ?: return null

        return DeviceLocation.of(location.latitude, location.longitude)
    }

    private fun isFresh(location: Location): Boolean =
        System.currentTimeMillis() - location.time <= FRESH_LAST_KNOWN_MAX_AGE_MILLIS

    private fun lastKnownLocationCandidates(locationManager: LocationManager): List<Location> =
        locationManager.allProviders
            .filterNot { it == LocationManager.PASSIVE_PROVIDER }
            .mapNotNull { provider ->
                runCatching { locationManager.getLastKnownLocation(provider) }.getOrNull()
            }

    /**
     * Requests a single fix from every enabled provider and resumes with the first one
     * that arrives; all registrations are removed as soon as one fix is delivered.
     */
    @SuppressLint("MissingPermission")
    @Suppress("DEPRECATION")
    private suspend fun awaitSingleFix(locationManager: LocationManager): Location? {
        val enabledProviders = locationManager.getProviders(true)
            .filterNot { it == LocationManager.PASSIVE_PROVIDER }
        if (enabledProviders.isEmpty()) {
            return null
        }

        return suspendCancellableCoroutine { continuation ->
            val listener = object : LocationListener {
                override fun onLocationChanged(location: Location) {
                    if (continuation.isActive) {
                        runCatching { locationManager.removeUpdates(this) }
                        continuation.resume(location)
                    }
                }
            }
            enabledProviders.forEach { provider ->
                runCatching {
                    locationManager.requestSingleUpdate(provider, listener, Looper.getMainLooper())
                }
            }
            continuation.invokeOnCancellation {
                runCatching { locationManager.removeUpdates(listener) }
            }
        }
    }

    private companion object {
        /**
         * Cached fixes older than this are treated as stale and a fresh fix is requested.
         */
        const val FRESH_LAST_KNOWN_MAX_AGE_MILLIS = 2 * 60 * 1000L

        /**
         * Upper bound for waiting on a fresh fix before giving up (UC-015 must not hang).
         */
        const val DEFAULT_FIX_TIMEOUT_MILLIS = 8_000L
    }
}
