package com.anpfuel.app.ui.enterprise.tracking

import android.Manifest
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import android.location.Location
import android.location.LocationManager
import android.os.Build
import android.os.IBinder
import android.os.BatteryManager
import android.os.Looper
import android.os.SystemClock
import android.provider.Settings
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import androidx.core.location.LocationCompat
import androidx.core.location.LocationListenerCompat
import androidx.core.location.LocationManagerCompat
import androidx.core.location.LocationRequestCompat
import com.anpfuel.app.R
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueSource
import com.anpfuel.application.enterprise.tracking.EnterpriseTrackingService
import com.anpfuel.application.enterprise.tracking.TrackingBeginResult
import com.anpfuel.application.enterprise.tracking.TrackingCollector
import com.anpfuel.application.enterprise.tracking.TrackingFixSample
import com.anpfuel.application.enterprise.tracking.TrackingPlatform
import com.anpfuel.application.enterprise.tracking.TrackingRuntime
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.tracking.LocationPermission
import com.anpfuel.domain.enterprise.tracking.TrackingDevice
import com.anpfuel.domain.enterprise.tracking.TrackingGapReason
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

/*
 * EMP-GPS-002 organization trip tracking (B-BR-EMP-496..500, EDR-323): the fourth location foreground service. It is
 * never sticky, has no boot receiver, never asks for background location and listens only after consuming the one-time
 * grant that the visible trip screen armed for this exact partition and session. One listener; a 30-second watchdog.
 */

/** The process's installed tracking dependencies; the application (or a device test) installs them once. */
object OrganizationTrackingHost {
    class Installed(val runtime: TrackingRuntime, val service: EnterpriseTrackingService, val source: EnterpriseQueueSource,
        /** EMP-GPS-003: one delivery of the partition after batches were sealed (the ordinary queue). */
        val deliver: suspend (EnterpriseQueueAccountLease, EnterprisePartition) -> Unit = { _, _ -> }) {
        override fun toString(): String = "OrganizationTrackingHost.Installed(redacted)"
    }
    @Volatile var installed: Installed? = null
}

/** The Android platform of the collector: the one LocationManager subscription of this service. */
internal class AndroidTrackingPlatform(private val context: Context) : TrackingPlatform {
    private val manager: LocationManager? = context.getSystemService(LocationManager::class.java)
    private var active: LocationListenerCompat? = null

    override fun permission(): LocationPermission = when {
        ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED -> LocationPermission.PRECISE
        ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_COARSE_LOCATION) == PackageManager.PERMISSION_GRANTED -> LocationPermission.APPROXIMATE
        else -> LocationPermission.NONE
    }

    override fun notificationVisible(): Boolean = OrganizationTrackingNotifications.visible(context)

    override fun bootId(): String = "boot-" + Settings.Global.getInt(context.contentResolver, Settings.Global.BOOT_COUNT, 0)

    override fun attach(precision: LocationPermission, sink: (TrackingFixSample) -> Unit): Boolean {
        val manager = manager ?: return false
        if (active != null || precision == LocationPermission.NONE) return false
        val provider = if (precision == LocationPermission.PRECISE) LocationManager.GPS_PROVIDER else coarseProvider(manager)
        val request = LocationRequestCompat.Builder(5_000L).setMinUpdateIntervalMillis(5_000L)
            .setQuality(if (precision == LocationPermission.PRECISE) LocationRequestCompat.QUALITY_HIGH_ACCURACY else LocationRequestCompat.QUALITY_BALANCED_POWER_ACCURACY)
            .build()
        val listener = LocationListenerCompat { location -> sink(sample(location)) }
        active = listener
        return try {
            LocationManagerCompat.requestLocationUpdates(manager, provider, request, listener, Looper.getMainLooper())
            true
        } catch (_: SecurityException) { active = null; false } catch (_: IllegalArgumentException) { active = null; false }
    }

    override fun detach() {
        val listener = active ?: return
        active = null
        try { manager?.let { LocationManagerCompat.removeUpdates(it, listener) } } catch (_: SecurityException) { }
    }

    override fun wallMillis(): Long = System.currentTimeMillis()
    override fun elapsedMillis(): Long = SystemClock.elapsedRealtime()

    /** BR-GPS-019: below 15 % and not charging. */
    override fun batteryLow(): Boolean {
        val battery = context.getSystemService(BatteryManager::class.java) ?: return false
        val level = battery.getIntProperty(BatteryManager.BATTERY_PROPERTY_CAPACITY)
        return level in 0 until com.anpfuel.domain.enterprise.tracking.TrackingSamplingPolicy.ECONOMY_BATTERY_PERCENT && !battery.isCharging
    }

    override fun providerEnabled(precision: LocationPermission): Boolean {
        val manager = manager ?: return false
        if (!LocationManagerCompat.isLocationEnabled(manager)) return false
        return try { manager.isProviderEnabled(if (precision == LocationPermission.PRECISE) LocationManager.GPS_PROVIDER else coarseProvider(manager)) }
        catch (_: IllegalArgumentException) { false }
    }

    private fun coarseProvider(manager: LocationManager): String =
        if (Build.VERSION.SDK_INT >= 31 && manager.allProviders.contains(LocationManager.FUSED_PROVIDER)) LocationManager.FUSED_PROVIDER
        else LocationManager.NETWORK_PROVIDER

    private fun sample(location: Location) = TrackingFixSample(
        location.latitude, location.longitude, if (location.hasAccuracy()) location.accuracy else null,
        if (location.hasSpeed()) location.speed else null, if (location.hasBearing()) location.bearing else null,
        location.time, location.elapsedRealtimeNanos / 1_000_000, LocationCompat.isMock(location),
    )
}

/** The tracking notification channel and the visibility check of BR-GPS-004. */
object OrganizationTrackingNotifications {
    const val CHANNEL_ID = "enterprise-trip-tracking"
    const val NOTIFICATION_ID = 4802

    fun ensureChannel(context: Context) {
        context.getSystemService(NotificationManager::class.java)?.let { manager ->
            if (manager.getNotificationChannel(CHANNEL_ID) == null) manager.createNotificationChannel(
                NotificationChannel(CHANNEL_ID, context.getString(R.string.enterprise_tracking_notification_channel), NotificationManager.IMPORTANCE_LOW))
        }
    }

    /** Recording needs the notification to be seen: the app's notifications and this channel must be enabled. */
    fun visible(context: Context): Boolean {
        ensureChannel(context)
        val compat = NotificationManagerCompat.from(context)
        if (!compat.areNotificationsEnabled()) return false
        val channel = context.getSystemService(NotificationManager::class.java)?.getNotificationChannel(CHANNEL_ID) ?: return false
        return channel.importance != NotificationManager.IMPORTANCE_NONE
    }
}

class OrganizationTrackingService : Service() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private var collector: TrackingCollector? = null
    private var platform: AndroidTrackingPlatform? = null
    private var watchdog: Job? = null
    private var consumer: Job? = null
    /** EMP-GPS-010: follows the collector's open gap to keep the ongoing notification truthful. */
    private var notices: Job? = null
    private var name: String = ""

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_START -> begin(intent)
            ACTION_STOP -> scope.launch { collector?.stopByDriver(); shutdown() }
            ACTION_RELEASE -> scope.launch { collector?.release(); shutdown() }
            else -> if (collector?.runningSessionId == null) shutdown()
        }
        // Never restarted by the system: a dead process is an interruption found at the next opening (BR-GPS-011).
        return START_NOT_STICKY
    }

    private fun begin(intent: Intent) {
        val partition = try {
            EnterprisePartition.of(intent.getStringExtra(EXTRA_ACCOUNT).orEmpty(), intent.getStringExtra(EXTRA_ORIGIN).orEmpty(),
                intent.getStringExtra(EXTRA_ORGANIZATION).orEmpty())
        } catch (_: Exception) { null }
        val sessionId = intent.getStringExtra(EXTRA_SESSION).orEmpty()
        name = intent.getStringExtra(EXTRA_NAME).orEmpty().take(120)
        // A foreground service must show its notification at once; a refused start removes it again.
        val shown = runCatching {
            OrganizationTrackingNotifications.ensureChannel(this)
            ServiceCompat.startForeground(this, OrganizationTrackingNotifications.NOTIFICATION_ID, notification(name),
                if (Build.VERSION.SDK_INT >= 29) ServiceInfo.FOREGROUND_SERVICE_TYPE_LOCATION else 0)
        }.isSuccess
        val installed = OrganizationTrackingHost.installed
        if (!shown || partition == null || installed == null) {
            // EMP-GPS-010 (B-BR-EMP-544): a session the visible screen granted that cannot run in the foreground is interrupted
            // at once with the observed cause, never left shown as recording; without that grant nothing is written.
            if (installed != null && partition != null && collector?.runningSessionId == null && installed.runtime.gate.consume(partition, sessionId)) {
                val observed = AndroidTrackingPlatform(this)
                scope.launch {
                    runCatching {
                        installed.service.interruptUncollected(installed.source.account(), partition, sessionId, observed.bootId(),
                            TrackingDevice(observed.permission(), observed.notificationVisible()))
                    }
                    shutdown()
                }
            } else if (collector?.runningSessionId == null) shutdown()
            return
        }
        val active = collector ?: TrackingCollector(installed.service, installed.source, installed.runtime.gate,
            AndroidTrackingPlatform(this).also { platform = it }, installed.deliver).also { collector = it }
        scope.launch {
            when (active.begin(partition, sessionId)) {
                TrackingBeginResult.STARTED -> {
                    installed.runtime.collector = active
                    // EMP-GPS-003: reports are recorded durably, in arrival order, off the platform callback.
                    if (consumer == null) consumer = scope.launch { active.run() }
                    // EMP-GPS-010: location off, no signal or only simulated locations are said in the ongoing notification.
                    notices?.cancel()
                    notices = scope.launch { active.state.map { it.gap }.distinctUntilChanged().drop(1).collect(::showGap) }
                    startWatchdog(active)
                }
                TrackingBeginResult.ALREADY_RUNNING -> Unit
                else -> if (active.runningSessionId == null) shutdown()
            }
        }
    }

    private fun startWatchdog(active: TrackingCollector) {
        watchdog?.cancel()
        watchdog = scope.launch {
            while (isActive) {
                delay(WATCHDOG_MILLIS)
                if (!active.tick()) { shutdown(); return@launch }
            }
        }
    }

    private fun showGap(gap: TrackingGapReason?) {
        if (!OrganizationTrackingNotifications.visible(this)) return
        runCatching { getSystemService(NotificationManager::class.java)?.notify(OrganizationTrackingNotifications.NOTIFICATION_ID, notification(name, gap)) }
    }

    private fun shutdown() {
        watchdog?.cancel(); watchdog = null
        consumer?.cancel(); consumer = null
        notices?.cancel(); notices = null
        val installed = OrganizationTrackingHost.installed
        if (installed?.runtime?.collector === collector) installed?.runtime?.collector = null
        runCatching { ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE) }
        stopSelf()
    }

    private fun notification(name: String, gap: TrackingGapReason? = null): Notification {
        val stop = PendingIntent.getService(this, 0, Intent(this, OrganizationTrackingService::class.java).setAction(ACTION_STOP),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        val open = packageManager.getLaunchIntentForPackage(packageName)?.let {
            PendingIntent.getActivity(this, 1, it, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        }
        return NotificationCompat.Builder(this, OrganizationTrackingNotifications.CHANNEL_ID)
            .setSmallIcon(android.R.drawable.ic_menu_mylocation)
            .setContentTitle(getString(R.string.enterprise_tracking_notification_title, name))
            .setContentText(getString(when (gap) {
                TrackingGapReason.PROVIDER_DISABLED -> R.string.enterprise_tracking_notification_gap_provider
                TrackingGapReason.SIGNAL_LOST -> R.string.enterprise_tracking_notification_gap_signal
                TrackingGapReason.MOCK_LOCATION -> R.string.enterprise_tracking_notification_gap_mock
                null -> R.string.enterprise_tracking_notification_text
            }))
            .setOnlyAlertOnce(true)
            .setOngoing(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)
            .apply { open?.let(::setContentIntent) }
            .addAction(0, getString(R.string.enterprise_tracking_stop), stop)
            .build()
    }

    override fun onDestroy() {
        watchdog?.cancel()
        val active = collector
        collector = null
        val installed = OrganizationTrackingHost.installed
        if (installed?.runtime?.collector === active) installed?.runtime?.collector = null
        // Releases only this service's listener; nothing is written on the way out.
        platform?.detach()
        scope.cancel()
        super.onDestroy()
    }

    companion object {
        const val ACTION_START = "com.anpfuel.action.ORGANIZATION_TRACKING_START"
        const val ACTION_STOP = "com.anpfuel.action.ORGANIZATION_TRACKING_STOP"
        const val ACTION_RELEASE = "com.anpfuel.action.ORGANIZATION_TRACKING_RELEASE"
        const val EXTRA_ACCOUNT = "account"
        const val EXTRA_ORIGIN = "origin"
        const val EXTRA_ORGANIZATION = "organization"
        const val EXTRA_SESSION = "session"
        const val EXTRA_NAME = "name"
        const val WATCHDOG_MILLIS = 30_000L

        fun startIntent(context: Context, partition: EnterprisePartition, sessionId: String, organizationName: String): Intent =
            Intent(context, OrganizationTrackingService::class.java).setAction(ACTION_START)
                .putExtra(EXTRA_ACCOUNT, partition.ownerAccount).putExtra(EXTRA_ORIGIN, partition.origin)
                .putExtra(EXTRA_ORGANIZATION, partition.organizationId).putExtra(EXTRA_SESSION, sessionId).putExtra(EXTRA_NAME, organizationName.take(120))
    }
}

/** What the trip screen asks of the service; a no-op when tracking is not wired (tests, rollback flag). */
interface TrackingServiceLauncher {
    /** Arms the one-time grant for this session and starts the service from the visible screen. */
    fun launch(partition: EnterprisePartition, sessionId: String, organizationName: String)
    /** Releases a running collector of this session without writing. */
    fun release(sessionId: String)
    companion object {
        val NONE: TrackingServiceLauncher = object : TrackingServiceLauncher {
            override fun launch(partition: EnterprisePartition, sessionId: String, organizationName: String) = Unit
            override fun release(sessionId: String) = Unit
        }
    }
}

class AndroidTrackingServiceLauncher(private val context: Context, private val runtime: TrackingRuntime) : TrackingServiceLauncher {
    override fun launch(partition: EnterprisePartition, sessionId: String, organizationName: String) {
        runtime.gate.arm(partition, sessionId)
        runCatching { ContextCompat.startForegroundService(context, OrganizationTrackingService.startIntent(context, partition, sessionId, organizationName)) }
    }
    override fun release(sessionId: String) {
        if (!runtime.running(sessionId)) return
        runCatching { context.startService(Intent(context, OrganizationTrackingService::class.java).setAction(OrganizationTrackingService.ACTION_RELEASE)) }
    }
    override fun toString(): String = "AndroidTrackingServiceLauncher(redacted)"
}
