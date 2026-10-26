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
import androidx.core.app.NotificationCompat
import androidx.core.content.ContextCompat
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.local.entity.DriverGpsObservationEntity
import com.anpfuel.data.repository.DriverLocalStore
import dagger.hilt.android.AndroidEntryPoint
import java.util.UUID
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
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private var manager: LocationManager? = null
    private var listener: LocationListener? = null
    private var shiftId = ""
    private var startedAt = 0L
    private var metres = 0.0
    private var last: Location? = null

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_START -> begin(intent.getStringExtra(EXTRA_SHIFT).orEmpty())
            ACTION_STOP -> finish()
            else -> stopSelf()
        }
        return START_NOT_STICKY
    }

    private fun begin(id: String) {
        if (id.isBlank()) return stopSelf()
        startForeground(NOTIFICATION_ID, notification())
        if (ContextCompat.checkSelfPermission(this, Manifest.permission.ACCESS_FINE_LOCATION) != PackageManager.PERMISSION_GRANTED) {
            stopSelf(); return
        }
        val lm = getSystemService(LocationManager::class.java) ?: run { stopSelf(); return }
        if (!lm.isProviderEnabled(LocationManager.GPS_PROVIDER)) { stopSelf(); return }
        shiftId = id; startedAt = System.currentTimeMillis(); metres = 0.0; last = null
        val active = object : LocationListener {
            override fun onLocationChanged(location: Location) {
                if (location.accuracy > 50f) return
                val previous = last
                if (previous != null) metres += previous.distanceTo(location).toDouble()
                last = location
            }
            @Deprecated("Deprecated in Java") override fun onStatusChanged(provider: String?, status: Int, extras: Bundle?) = Unit
        }
        listener = active; manager = lm
        try {
            lm.requestLocationUpdates(LocationManager.GPS_PROVIDER, 2000L, 0f, active, Looper.getMainLooper())
        } catch (_: SecurityException) { stopSelf() }
    }

    private fun finish() {
        manager?.removeUpdates(listener ?: return stopSelf())
        val owner = sessions.load()?.accountId ?: "local"
        val now = System.currentTimeMillis()
        scope.launch {
            store.saveGpsObservation(
                DriverGpsObservationEntity(owner, "android", UUID.randomUUID().toString(), shiftId, startedAt, now,
                    metres.toLong(), true, false, now),
            )
        }
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
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
        if (Build.VERSION.SDK_INT >= 26) {
            getSystemService(NotificationManager::class.java)?.createNotificationChannel(
                NotificationChannel(CHANNEL, "GPS do turno", NotificationManager.IMPORTANCE_LOW),
            )
        }
    }

    override fun onDestroy() { scope.cancel(); super.onDestroy() }

    companion object {
        private const val CHANNEL = "driver-gps"
        private const val NOTIFICATION_ID = 7711
        const val ACTION_START = "com.anpfuel.action.DRIVER_GPS_START"
        const val ACTION_STOP = "com.anpfuel.action.DRIVER_GPS_STOP"
        const val EXTRA_SHIFT = "shift_id"
        fun start(context: Context, shiftId: String) {
            ContextCompat.startForegroundService(context, Intent(context, DriverGpsService::class.java).setAction(ACTION_START).putExtra(EXTRA_SHIFT, shiftId))
        }
        fun stop(context: Context) { context.startService(Intent(context, DriverGpsService::class.java).setAction(ACTION_STOP)) }
    }
}
