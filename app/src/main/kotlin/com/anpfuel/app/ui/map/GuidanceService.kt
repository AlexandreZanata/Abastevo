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
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.os.SystemClock
import androidx.core.app.NotificationCompat
import androidx.core.content.ContextCompat
import com.anpfuel.app.R
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.guidance.GuidanceRun
import com.anpfuel.application.usecase.guidance.GuidanceRuntime
import com.anpfuel.domain.routing.GuidancePolicy
import com.anpfuel.domain.routing.NativeRouteRequest
import com.anpfuel.domain.routing.NavigationFix
import com.anpfuel.domain.routing.NavigationFixPolicy
import com.anpfuel.domain.routing.RoadAvoidance
import com.anpfuel.domain.routing.RoutePoint
import com.anpfuel.maps.engine.MapEngine
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject

/**
 * Explicit foreground navigation: the single continuous GPS owner during
 * guidance. Browsing never starts this service. START_NOT_STICKY never
 * restarts GPS after process death; recovery requires explicit resume.
 */
@AndroidEntryPoint
class GuidanceService : Service() {
    @Inject lateinit var engine: MapEngine
    @Inject lateinit var sessions: AuthSessionStore

    private lateinit var recovery: GuidanceRecoveryStore
    private lateinit var voice: LocalGuidanceVoice
    private var run: GuidanceRun? = null
    private var activeOwner = ""
    private var activeRequest: NativeRouteRequest? = null
    private var locationManager: LocationManager? = null
    private var listener: LocationListener? = null
    private var lastFixNanos: Long = 0L
    private var gpsLostPosted = false
    private val mainHandler = Handler(Looper.getMainLooper())

    override fun onCreate() {
        super.onCreate()
        recovery = GuidanceRecoveryStore(this)
        voice = LocalGuidanceVoice(applicationContext)
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_STOP -> {
                stopGuidance("explicit-stop")
                stopSelf()
                return START_NOT_STICKY
            }
            ACTION_START -> {
                val owner = currentOwner()
                val request = intent.toRouteRequest() ?: run {
                    GuidanceServiceState.update { it.copy(active = false, error = "invalid") }
                    stopSelf()
                    return START_NOT_STICKY
                }
                startGuidance(owner, request)
                return START_NOT_STICKY
            }
            else -> {
                stopSelf()
                return START_NOT_STICKY
            }
        }
    }

    private fun currentOwner(): String {
        return try {
            sessions.load()?.accountId ?: "anonymous"
        } catch (_: Exception) {
            "anonymous"
        }
    }

    private fun startGuidance(owner: String, request: NativeRouteRequest) {
        // Foreground must be entered immediately; failures below stop explicitly.
        runCatching { startForegroundWithAction(request) }
        // Fence 1: precise foreground location, enabled GPS, visible notifications.
        if (ContextCompat.checkSelfPermission(this, Manifest.permission.ACCESS_FINE_LOCATION) != PackageManager.PERMISSION_GRANTED) {
            GuidanceServiceState.update { it.copy(active = false, error = "no-permission") }
            runCatching { stopForeground(STOP_FOREGROUND_REMOVE) }
            stopSelf()
            return
        }
        val manager = getSystemService(LocationManager::class.java) ?: run {
            GuidanceServiceState.update { it.copy(active = false, error = "gps-off") }
            runCatching { stopForeground(STOP_FOREGROUND_REMOVE) }
            stopSelf()
            return
        }
        if (!manager.isProviderEnabled(LocationManager.GPS_PROVIDER)) {
            GuidanceServiceState.update { it.copy(active = false, error = "gps-off") }
            runCatching { stopForeground(STOP_FOREGROUND_REMOVE) }
            stopSelf()
            return
        }
        if (Build.VERSION.SDK_INT >= 33) {
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
                GuidanceServiceState.update { it.copy(active = false, error = "notification-blocked") }
                runCatching { stopForeground(STOP_FOREGROUND_REMOVE) }
                stopSelf()
                return
            }
        }
        if (!isChannelVisible()) {
            GuidanceServiceState.update { it.copy(active = false, error = "notification-blocked") }
            runCatching { stopForeground(STOP_FOREGROUND_REMOVE) }
            stopSelf()
            return
        }
        // The service owns the single continuous subscription; the engine owns native following.
        // onStartCommand already runs on Main, so call directly there to avoid self-deadlock.
        val runtime = object : GuidanceRuntime {
            override fun startNative(): Boolean {
                if (android.os.Looper.myLooper() == android.os.Looper.getMainLooper()) {
                    return try {
                        engine.startGuidance(request)
                    } catch (_: Exception) {
                        false
                    }
                }
                var ok = false
                val latch = java.util.concurrent.CountDownLatch(1)
                mainHandler.post {
                    try {
                        ok = engine.startGuidance(request)
                    } finally {
                        latch.countDown()
                    }
                }
                latch.await(5, java.util.concurrent.TimeUnit.SECONDS)
                return ok
            }
            override fun stopNative() {
                if (android.os.Looper.myLooper() == android.os.Looper.getMainLooper()) {
                    runCatching { engine.stopGuidance() }
                    return
                }
                val latch = java.util.concurrent.CountDownLatch(1)
                mainHandler.post {
                    try {
                        engine.stopGuidance()
                    } finally {
                        latch.countDown()
                    }
                }
                latch.await(5, java.util.concurrent.TimeUnit.SECONDS)
            }
            override fun startLocation() = startLocationUpdates()
            override fun stopLocation() = stopLocationUpdates()
            override fun startVoice() {
                val started = voice.start(request.language, preferredEngine = null)
                GuidanceServiceState.update { it.copy(voiceUnavailable = !started) }
            }
            override fun pauseVoice() = voice.pause()
            override fun stopVoice() = voice.stop()
        }
        val next = GuidanceRun(owner, ::currentOwner, runtime)
        if (!next.start()) {
            GuidanceServiceState.update { it.copy(active = false, error = "start-refused") }
            runCatching { stopForeground(STOP_FOREGROUND_REMOVE) }
            stopSelf()
            return
        }
        run = next
        activeOwner = owner
        activeRequest = request
        lastFixNanos = 0L
        gpsLostPosted = false
        if (!recovery.save(owner, request)) {
            next.stop()
            run = null
            GuidanceServiceState.update { it.copy(active = false, error = "storage-failed") }
            runCatching { stopForeground(STOP_FOREGROUND_REMOVE) }
            stopSelf()
            return
        }
        GuidanceServiceState.update {
            it.copy(active = true, owner = owner, error = null, rerouteFailed = false, resumeAvailable = false)
        }
    }

    private fun isChannelVisible(): Boolean {
        return try {
            val manager = getSystemService(NotificationManager::class.java) ?: return true
            if (Build.VERSION.SDK_INT < 26) return true
            val channel = manager.getNotificationChannel(CHANNEL_ID)
            channel == null || channel.importance != NotificationManager.IMPORTANCE_NONE
        } catch (_: Exception) {
            true
        }
    }

    private fun startForegroundWithAction(request: NativeRouteRequest) {
        val manager = getSystemService(NotificationManager::class.java)
        if (Build.VERSION.SDK_INT >= 26) {
            runCatching {
                manager?.createNotificationChannel(
                    NotificationChannel(CHANNEL_ID, getString(R.string.map23_notification_title), NotificationManager.IMPORTANCE_LOW),
                )
            }
        }
        val stopIntent = Intent(this, GuidanceService::class.java).setAction(ACTION_STOP)
        val stopPending = PendingIntent.getService(this, 0, stopIntent, PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
        val notification: Notification = NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle(getString(R.string.map23_notification_title))
            .setContentText(request.points.lastOrNull()?.label ?: "")
            .setSmallIcon(android.R.drawable.ic_menu_mylocation)
            .setOngoing(true)
            .addAction(android.R.drawable.ic_menu_close_clear_cancel, getString(R.string.map23_stop_action), stopPending)
            .build()
        if (Build.VERSION.SDK_INT >= 29) {
            startForeground(NOTIFICATION_ID, notification, android.content.pm.ServiceInfo.FOREGROUND_SERVICE_TYPE_LOCATION)
        } else {
            startForeground(NOTIFICATION_ID, notification)
        }
    }

    private fun startLocationUpdates() {
        val manager = getSystemService(LocationManager::class.java) ?: throw SecurityException("location unavailable")
        locationManager = manager
        val active = object : LocationListener {
            override fun onLocationChanged(location: Location) = onFix(location)
            override fun onProviderDisabled(provider: String) {
                GuidanceServiceState.update { it.copy(gpsLost = true) }
            }
            override fun onProviderEnabled(provider: String) {
                GuidanceServiceState.update { it.copy(gpsLost = false) }
            }
            @Deprecated("Deprecated in Java")
            override fun onStatusChanged(provider: String?, status: Int, extras: Bundle?) = Unit
        }
        listener = active
        try {
            manager.requestLocationUpdates(LocationManager.GPS_PROVIDER, 1000L, 0f, active, Looper.getMainLooper())
        } catch (error: SecurityException) {
            throw error
        }
    }

    private fun stopLocationUpdates() {
        runCatching {
            val manager = locationManager ?: getSystemService(LocationManager::class.java)
            listener?.let { manager?.removeUpdates(it) }
        }
        listener = null
    }

    private fun onFix(location: Location) {
        val current = run ?: return
        // Owner/account switch synchronously fences fixes and stops resources.
        if (!current.checkOwner()) {
            stopGuidance("owner-changed")
            stopSelf()
            return
        }
        val nowNanos = SystemClock.elapsedRealtimeNanos()
        val ageNanos = nowNanos - location.elapsedRealtimeNanos
        val fix = NavigationFix(
            latitude = location.latitude,
            longitude = location.longitude,
            accuracyMeters = if (location.hasAccuracy()) location.accuracy else Float.NaN,
            timeMillis = location.time,
            elapsedRealtimeNanos = location.elapsedRealtimeNanos,
            altitudeMeters = if (location.hasAltitude()) location.altitude else 0.0,
            speedMetersPerSecond = if (location.hasSpeed()) location.speed else 0f,
            bearingDegrees = if (location.hasBearing()) location.bearing else 0f,
        )
        val accepted = current.accept(fix, nowNanos)
        if (!accepted) {
            val poor = !NavigationFixPolicy.accepts(fix, nowNanos)
            GuidanceServiceState.update { it.copy(gpsPoor = poor, progress = it.progress) }
            return
        }
        lastFixNanos = nowNanos
        gpsLostPosted = false
        GuidanceServiceState.update { it.copy(gpsPoor = false, gpsLost = false) }
        // Feed on Main: the pinned core requires it and recalculates internally.
        mainHandler.post {
            val feed = try {
                engine.feedGuidanceFix(fix, nowNanos)
            } catch (_: Exception) {
                null
            } ?: return@post
            GuidanceServiceState.update { state ->
                state.copy(
                    progress = feed.progress ?: state.progress,
                    notifications = feed.notifications,
                    recalculating = feed.progress?.nativeState == "RouteRebuilding" ||
                        feed.progress?.nativeState == "OffRoute" ||
                        feed.progress?.nativeState == "RouteNeedsRebuild",
                    rerouteFailed = feed.rerouteRecoverable,
                    gpsPoor = false,
                )
            }
            val first = feed.notifications.firstOrNull()
            if (first != null && !feed.rerouteRecoverable) {
                val locale = GuidancePolicy.nativeLocaleFor(activeRequest?.language ?: "en")
                if (locale != null) voice.speak(first)
            }
            if (feed.rerouteRecoverable) {
                GuidanceServiceState.update { it.copy(recalculating = false) }
            }
        }
    }

    private fun stopGuidance(reason: String) {
        runCatching { voice.stop() }
        runCatching { stopLocationUpdates() }
        runCatching { run?.stop() }
        run = null
        activeRequest = null
        // Explicit stop invalidates callbacks, audio, location, native and recovery.
        recovery.clear()
        mainHandler.post {
            runCatching { engine.stopGuidance() }
        }
        GuidanceServiceState.update {
            GuidanceUiState(active = false, resumeAvailable = false, error = if (reason == "explicit-stop" || reason == "owner-changed") null else reason)
        }
        runCatching { stopForeground(STOP_FOREGROUND_REMOVE) }
    }

    override fun onDestroy() {
        // Process death never restarts GPS: keep saved recovery for explicit resume.
        runCatching { voice.stop() }
        runCatching { stopLocationUpdates() }
        runCatching { run?.stop() }
        run = null
        super.onDestroy()
    }

    companion object {
        const val ACTION_START = "com.anpfuel.action.GUIDANCE_START"
        const val ACTION_STOP = "com.anpfuel.action.GUIDANCE_STOP"
        const val CHANNEL_ID = "guidance"
        const val NOTIFICATION_ID = 4201

        fun startIntent(context: Context, owner: String, request: NativeRouteRequest): Intent {
            return Intent(context, GuidanceService::class.java).setAction(ACTION_START)
                .putExtra("owner", owner)
                .putExtra("language", request.language)
                .putExtra("avoid", ArrayList(request.avoid.map { it.name }))
                .putExtra("lat", request.points.map { it.latitude }.toDoubleArray())
                .putExtra("lon", request.points.map { it.longitude }.toDoubleArray())
                .putExtra("label", ArrayList(request.points.map { it.label.take(512) }))
        }

        fun stopIntent(context: Context): Intent {
            return Intent(context, GuidanceService::class.java).setAction(ACTION_STOP)
        }

        private fun Intent.toRouteRequest(): NativeRouteRequest? {
            return try {
                val lat = getDoubleArrayExtra("lat") ?: return null
                val lon = getDoubleArrayExtra("lon") ?: return null
                val labels = getStringArrayListExtra("label") ?: ArrayList()
                val avoidNames = getStringArrayListExtra("avoid") ?: ArrayList()
                if (lat.size != lon.size || lat.size !in 2..102) return null
                val points = lat.indices.map { index ->
                    RoutePoint(lat[index], lon[index], labels.getOrElse(index) { "" })
                }
                val avoid = avoidNames.mapNotNull { runCatching { RoadAvoidance.valueOf(it) }.getOrNull() }.toSet()
                val language = getStringExtra("language") ?: "en"
                NativeRouteRequest(points, avoid, language).also { it.validate() }
            } catch (_: Exception) {
                null
            }
        }
    }
}
