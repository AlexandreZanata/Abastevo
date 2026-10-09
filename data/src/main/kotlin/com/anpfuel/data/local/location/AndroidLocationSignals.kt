package com.anpfuel.data.local.location

import android.Manifest
import android.content.Context
import android.location.Location
import android.location.LocationManager
import android.os.Build
import android.os.SystemClock
import androidx.core.content.ContextCompat
import com.anpfuel.application.portable.LocationSignal
import com.anpfuel.application.portable.LocationSignalSource

/**
 * Pure Android boundary reduction (P16-T02): raw provider values fold
 * into one [LocationSignal] without touching providers. Fully
 * JVM-testable; the thin provider wiring lives in
 * [AndroidLocationSignals]. Negative accuracy is invalid data, not a
 * precise fix: it folds to missing, and the frozen contract refuses
 * the claim downstream.
 */
object LocationReading {

    fun signal(
        permissionGranted: Boolean,
        locationPresent: Boolean,
        sourceInfoAvailable: Boolean,
        simulated: Boolean,
        accuracyMeters: Double?,
        fixAgeSeconds: Long?,
        clockSkewSeconds: Long? = null,
    ): LocationSignal {
        if (!permissionGranted) return LocationSignal.denied()
        if (!locationPresent) return LocationSignal.absent()
        val hasAccuracy = accuracyMeters != null && accuracyMeters >= 0.0
        return LocationSignal(
            permissionGranted = true,
            sourceInfoAvailable = sourceInfoAvailable,
            simulated = simulated,
            hasAccuracy = hasAccuracy,
            accuracyMeters = if (hasAccuracy) accuracyMeters!! else 0.0,
            fixAgeSeconds = fixAgeSeconds,
            clockSkewSeconds = clockSkewSeconds,
        )
    }
}

/**
 * One-shot Android location signals (P16-T02, L01…L03).
 *
 * Reads the best last-known fix once per [read] and reports the
 * framework mock flag (`Location.isMock` on API 31+,
 * `isFromMockProvider` below — the same OS-provided signals
 * `LocationCompat` wraps, without a new dependency). Never requests
 * location updates, never polls, never enables background tracking:
 * staleness is enforced by the frozen contract, not by repeated
 * reads. Reads no developer settings and keeps no blacklist: an
 * unknown source refuses by default. Clock skew stays null here;
 * the server computes receipt skew independently (P16-T03).
 *
 * The ~25 provider-wiring lines below cannot run on JVM unit tests
 * (no Android runtime): they are statically reviewed and
 * lint-guarded, while [LocationReading] and the portable flow carry
 * the behavioral suites.
 */
class AndroidLocationSignals(
    private val context: Context,
    private val nowNanos: () -> Long = { SystemClock.elapsedRealtimeNanos() },
) : LocationSignalSource {

    override fun read(): LocationSignal? {
        if (!hasPermission()) return LocationSignal.denied()
        val manager = context.getSystemService(LocationManager::class.java) ?: return null
        val location = try {
            bestLastKnown(manager)
        } catch (_: SecurityException) {
            return LocationSignal.denied()
        } catch (_: Exception) {
            return null
        } ?: return LocationSignal.absent()
        val simulated = if (Build.VERSION.SDK_INT >= 31) {
            location.isMock
        } else {
            @Suppress("DEPRECATION")
            location.isFromMockProvider
        }
        val ageSeconds = if (location.elapsedRealtimeNanos > 0) {
            (nowNanos() - location.elapsedRealtimeNanos) / 1_000_000_000L
        } else {
            null
        }
        return LocationReading.signal(
            permissionGranted = true,
            locationPresent = true,
            // The framework always exposes the mock flag on real fixes.
            sourceInfoAvailable = true,
            simulated = simulated,
            accuracyMeters = if (location.hasAccuracy()) location.accuracy.toDouble() else null,
            fixAgeSeconds = ageSeconds,
        )
    }

    private fun hasPermission(): Boolean {
        val fine = ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION)
        val coarse = ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_COARSE_LOCATION)
        return fine == android.content.pm.PackageManager.PERMISSION_GRANTED ||
            coarse == android.content.pm.PackageManager.PERMISSION_GRANTED
    }

    private fun bestLastKnown(manager: LocationManager): Location? {
        var best: Location? = null
        for (provider in manager.getProviders(true)) {
            // SecurityException propagates to read(), which maps it to
            // denied; any other provider failure skips that provider.
            val candidate = try {
                manager.getLastKnownLocation(provider)
            } catch (denied: SecurityException) {
                throw denied
            } catch (_: Exception) {
                continue
            } ?: continue
            if (best == null || candidate.elapsedRealtimeNanos > best.elapsedRealtimeNanos) {
                best = candidate
            }
        }
        return best
    }
}
