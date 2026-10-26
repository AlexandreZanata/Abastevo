package com.anpfuel.app.ui.map

import android.Manifest
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.location.Location
import android.location.LocationListener
import android.location.LocationManager
import android.os.Build
import android.os.Bundle
import android.os.IBinder
import android.os.Looper
import android.os.SystemClock
import androidx.core.app.NotificationCompat
import androidx.core.content.ContextCompat
import com.anpfuel.app.R
import com.anpfuel.application.usecase.mapsaved.TrackRecordingUseCase
import com.anpfuel.domain.routing.NavigationFix
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** Observable recorder state; contains counts only, never coordinates. */
data class TripRecordingUiState(val active: Boolean = false, val points: Int = 0, val error: String? = null)

object TripRecordingServiceState {
    private val _state = MutableStateFlow(TripRecordingUiState())
    val state: StateFlow<TripRecordingUiState> = _state.asStateFlow()
    fun update(block: (TripRecordingUiState) -> TripRecordingUiState) = _state.update(block)
}

/**
 * MAP-25 explicit track recording (B-BR-MAP25-05). Only an explicit start intent (after the user
 * chose to record) with precise foreground location, enabled GPS and a visible notification starts
 * GPS; the notification offers Stop. Not sticky: process death never restarts recording, the
 * interrupted recording row is offered for explicit save/discard. Fixes go through
 * [TrackRecordingUseCase], which gates quality, spacing, limits and the owner.
 */
@AndroidEntryPoint
class TripRecordingService : Service() {
    @Inject lateinit var recorder: TrackRecordingUseCase

    private var listener: LocationListener? = null
    private var manager: LocationManager? = null
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_STOP -> finish(save = intent.getBooleanExtra(EXTRA_SAVE, true))
            ACTION_START -> begin(intent.getStringExtra(EXTRA_NAME).orEmpty())
            else -> stopSelf()
        }
        return START_NOT_STICKY
    }

    private fun fail(code: String) {
        TripRecordingServiceState.update { TripRecordingUiState(active = false, error = code) }
        runCatching { stopForeground(STOP_FOREGROUND_REMOVE) }
        stopSelf()
    }

    private fun begin(name: String) {
        runCatching { startForeground(NOTIFICATION_ID, notification()) }
        if (ContextCompat.checkSelfPermission(this, Manifest.permission.ACCESS_FINE_LOCATION) != PackageManager.PERMISSION_GRANTED) return fail("no-permission")
        val locationManager = getSystemService(LocationManager::class.java) ?: return fail("gps-off")
        if (!locationManager.isProviderEnabled(LocationManager.GPS_PROVIDER)) return fail("gps-off")
        if (Build.VERSION.SDK_INT >= 33 &&
            ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) return fail("notification-blocked")
        if (GuidanceServiceState.current().active) return fail("guidance-active")
        scope.launch {
            val started = try { recorder.start(name) } catch (_: Exception) { false }
            if (!started) {
                // A recording already exists: resume only by explicit stop/discard first.
                fail("recording-exists")
                return@launch
            }
            android.os.Handler(Looper.getMainLooper()).post { subscribe(locationManager) }
        }
    }

    private fun subscribe(locationManager: LocationManager) {
        manager = locationManager
        val active = object : LocationListener {
            override fun onLocationChanged(location: Location) = onFix(location)
            override fun onProviderDisabled(provider: String) {
                TripRecordingServiceState.update { it.copy(error = "gps-off") }
            }
            override fun onProviderEnabled(provider: String) {
                TripRecordingServiceState.update { it.copy(error = null) }
            }
            @Deprecated("Deprecated in Java")
            override fun onStatusChanged(provider: String?, status: Int, extras: Bundle?) = Unit
        }
        listener = active
        try {
            locationManager.requestLocationUpdates(LocationManager.GPS_PROVIDER, 2000L, 0f, active, Looper.getMainLooper())
            TripRecordingServiceState.update { TripRecordingUiState(active = true) }
        } catch (_: SecurityException) {
            fail("no-permission")
        }
    }

    private fun onFix(location: Location) {
        val now = SystemClock.elapsedRealtimeNanos()
        val fix = NavigationFix(
            location.latitude, location.longitude,
            if (location.hasAccuracy()) location.accuracy else Float.NaN,
            location.time, location.elapsedRealtimeNanos,
            if (location.hasAltitude()) location.altitude else 0.0,
        )
        scope.launch {
            try {
                recorder.onFix(fix, now)?.let { count -> TripRecordingServiceState.update { it.copy(points = count) } }
            } catch (_: Exception) {
                // Owner switched or storage failed: stop GPS; the row stays for explicit save/discard by its owner.
                android.os.Handler(Looper.getMainLooper()).post { stopGps(); fail("owner-changed") }
            }
        }
    }

    private fun stopGps() {
        runCatching { listener?.let { manager?.removeUpdates(it) } }
        listener = null
    }

    private fun finish(save: Boolean) {
        stopGps()
        // The storage work must complete before the service is torn down; otherwise onDestroy
        // cancels the scope and an explicit save or discard is silently lost.
        scope.launch {
            runCatching { recorder.stop(save) }
            android.os.Handler(Looper.getMainLooper()).post {
                TripRecordingServiceState.update { TripRecordingUiState() }
                runCatching { stopForeground(STOP_FOREGROUND_REMOVE) }
                stopSelf()
            }
        }
    }

    private fun notification(): Notification {
        val manager = getSystemService(NotificationManager::class.java)
        if (Build.VERSION.SDK_INT >= 26) {
            manager.createNotificationChannel(NotificationChannel(CHANNEL_ID, getString(R.string.map25_recording_channel), NotificationManager.IMPORTANCE_LOW))
        }
        val stop = PendingIntent.getService(
            this, 0, stopIntent(this, save = true),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(android.R.drawable.ic_menu_mylocation)
            .setContentTitle(getString(R.string.map25_recording_notification))
            .setOngoing(true)
            .addAction(0, getString(R.string.map25_recording_stop), stop)
            .build()
    }

    override fun onDestroy() {
        stopGps()
        scope.cancel()
        TripRecordingServiceState.update { it.copy(active = false) }
        super.onDestroy()
    }

    companion object {
        const val ACTION_START = "com.anpfuel.action.TRIP_RECORDING_START"
        const val ACTION_STOP = "com.anpfuel.action.TRIP_RECORDING_STOP"
        const val EXTRA_NAME = "name"
        const val EXTRA_SAVE = "save"
        const val CHANNEL_ID = "trip-recording"
        const val NOTIFICATION_ID = 4202

        fun startIntent(context: Context, name: String) =
            Intent(context, TripRecordingService::class.java).setAction(ACTION_START).putExtra(EXTRA_NAME, name.take(120))

        fun stopIntent(context: Context, save: Boolean) =
            Intent(context, TripRecordingService::class.java).setAction(ACTION_STOP).putExtra(EXTRA_SAVE, save)
    }
}
