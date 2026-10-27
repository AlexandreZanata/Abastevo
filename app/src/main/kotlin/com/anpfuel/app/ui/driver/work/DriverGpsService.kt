package com.anpfuel.app.ui.driver.work

import android.Manifest
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
import android.os.Handler
import androidx.core.app.NotificationCompat
import androidx.core.content.ContextCompat
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.repository.DriverLocalStore
import dagger.hilt.android.AndroidEntryPoint
import java.util.UUID
import java.lang.ref.WeakReference
import javax.inject.Inject
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch

/** DRV-21 explicit optional GPS while a manual shift is active. Never auto-starts. */
@AndroidEntryPoint
class DriverGpsService : Service() {
    @Inject lateinit var store: DriverLocalStore
    @Inject lateinit var sessions: AuthSessionStore
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private var manager: LocationManager? = null
    private var listener: LocationListener? = null
    private var last: Location? = null
    private var currentStartId = 0
    private val lifecycle by lazy {
        DriverGpsLifecycle(
            currentOwner = { sessions.load()?.accountId ?: "local" },
            eligible = { owner, shift ->
                val row = store.currentShiftChecked(owner, "android")
                row != null && row.shiftId == shift && row.state == "RUNNING"
            },
            persist = { store.saveGpsObservation(it) },
            attach = ::attachLocation,
            detach = ::detachLocation,
        )
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        currentStartId = startId
        when (intent?.action) {
            ACTION_START -> begin(intent.getStringExtra(EXTRA_SHIFT).orEmpty(), startId)
            ACTION_STOP -> finish(startId)
            else -> stopSelfResult(startId)
        }
        return START_NOT_STICKY
    }

    private fun begin(id: String, startId: Int) {
        if (id.isBlank()) { stopSelfResult(startId); return }
        startForeground(NOTIFICATION_ID, notification())
        if (ContextCompat.checkSelfPermission(this, Manifest.permission.ACCESS_FINE_LOCATION) != PackageManager.PERMISSION_GRANTED) {
            finish(startId); return
        }
        val lm = getSystemService(LocationManager::class.java) ?: run { finish(startId); return }
        if (!lm.isProviderEnabled(LocationManager.GPS_PROVIDER)) { finish(startId); return }
        scope.launch {
            val owner = sessions.load()?.accountId ?: "local"
            try {
                val started = lifecycle.start(owner, id, UUID.randomUUID().toString(), System.currentTimeMillis())
                if (!started && !lifecycle.isActive) stopSelfResult(startId)
            } catch (_: Exception) {
                if (!lifecycle.isActive) stopSelfResult(startId)
            }
        }
    }

    private fun attachLocation() {
        val lm = getSystemService(LocationManager::class.java) ?: error("location-manager-unavailable")
        last = null
        val active = object : LocationListener {
            override fun onLocationChanged(location: Location) {
                if (!lifecycle.addMetres(0.0)) { finish(currentStartId); return }
                if (location.accuracy > 50f) return
                val previous = last
                if (previous != null && !lifecycle.addMetres(previous.distanceTo(location).toDouble())) {
                    finish(currentStartId); return
                }
                last = location
            }
            override fun onProviderDisabled(provider: String) { finish(currentStartId) }
            @Deprecated("Deprecated in Java") override fun onStatusChanged(provider: String?, status: Int, extras: Bundle?) = Unit
        }
        listener = active; manager = lm
        lm.requestLocationUpdates(LocationManager.GPS_PROVIDER, 2000L, 0f, active, Looper.getMainLooper())
    }

    private fun detachLocation() {
        val active = listener
        listener = null
        last = null
        try { if (active != null) manager?.removeUpdates(active) } catch (_: SecurityException) { }
        manager = null
    }

    private fun finish(startId: Int) {
        scope.launch {
            try { lifecycle.stop(System.currentTimeMillis()) } catch (_: Exception) {
                // The already-persisted incomplete checkpoint remains available.
            }
            if (stopSelfResult(startId)) stopForeground(STOP_FOREGROUND_REMOVE)
        }
    }

    private fun notification() = NotificationCompat.Builder(this, CHANNEL)
        .setSmallIcon(android.R.drawable.ic_menu_mylocation)
        .setContentTitle("GPS do turno ativo")
        .setContentText("Toque em parar para encerrar a gravação opcional.")
        .setOngoing(true)
        .addAction(0, "Parar", PendingIntent.getService(this, 0, Intent(this, DriverGpsService::class.java).setAction(ACTION_STOP), PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE))
        .build()

    override fun onCreate() {
        super.onCreate()
        activeService = WeakReference(this)
        if (Build.VERSION.SDK_INT >= 26) {
            getSystemService(NotificationManager::class.java)?.createNotificationChannel(
                NotificationChannel(CHANNEL, "GPS do turno", NotificationManager.IMPORTANCE_LOW),
            )
        }
    }

    override fun onDestroy() {
        if (activeService?.get() === this) activeService = null
        lifecycle.destroy()
        scope.cancel()
        super.onDestroy()
    }

    companion object {
        private const val CHANNEL = "driver-gps"
        private const val NOTIFICATION_ID = 7711
        const val ACTION_START = "com.anpfuel.action.DRIVER_GPS_START"
        const val ACTION_STOP = "com.anpfuel.action.DRIVER_GPS_STOP"
        const val EXTRA_SHIFT = "shift_id"
        @Volatile private var activeService: WeakReference<DriverGpsService>? = null
        fun start(context: Context, shiftId: String) {
            ContextCompat.startForegroundService(context, Intent(context, DriverGpsService::class.java).setAction(ACTION_START).putExtra(EXTRA_SHIFT, shiftId))
        }
        fun stop(context: Context) {
            // No new background-service launch is needed when no recording exists.
            Handler(Looper.getMainLooper()).post { activeService?.get()?.let { it.finish(it.currentStartId) } }
        }
    }
}
