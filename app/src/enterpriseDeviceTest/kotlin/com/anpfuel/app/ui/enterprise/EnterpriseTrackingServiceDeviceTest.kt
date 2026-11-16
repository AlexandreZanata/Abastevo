package com.anpfuel.app.ui.enterprise

import android.Manifest
import android.app.NotificationManager
import android.content.Context
import android.content.Intent
import androidx.activity.ComponentActivity
import androidx.core.content.ContextCompat
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.R
import com.anpfuel.app.ui.enterprise.tracking.OrganizationTrackingHost
import com.anpfuel.app.ui.enterprise.tracking.OrganizationTrackingNotifications
import com.anpfuel.app.ui.enterprise.tracking.OrganizationTrackingService
import com.anpfuel.application.enterprise.EnterpriseAccessResult
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.EnterpriseDeviceTarget
import com.anpfuel.application.enterprise.EnterpriseEnrollmentBinding
import com.anpfuel.application.enterprise.EnterpriseLocalIdentity
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.queue.EnterpriseQueueProducer
import com.anpfuel.application.enterprise.queue.EnterpriseQueueSource
import com.anpfuel.application.enterprise.queue.EnterpriseQueueTransaction
import com.anpfuel.application.enterprise.tracking.EnterpriseTrackingProducers
import com.anpfuel.application.enterprise.tracking.EnterpriseTrackingService
import com.anpfuel.application.enterprise.tracking.EnterpriseTrackingSnapshot
import com.anpfuel.application.enterprise.tracking.EnterpriseTrackingStore
import com.anpfuel.application.enterprise.tracking.TrackingRuntime
import com.anpfuel.application.enterprise.tracking.TrackingStartGate
import com.anpfuel.application.enterprise.trip.EnterpriseTripProducers
import com.anpfuel.application.enterprise.trip.EnterpriseTripService
import com.anpfuel.application.enterprise.trip.EnterpriseTripSnapshot
import com.anpfuel.application.enterprise.trip.EnterpriseTripStartHints
import com.anpfuel.application.enterprise.trip.TripCapture
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.fleet.AssetKind
import com.anpfuel.domain.enterprise.fleet.EnterpriseAssetChoice
import com.anpfuel.domain.enterprise.fleet.UsageBasis
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.tracking.LocationPermission
import com.anpfuel.domain.enterprise.tracking.SessionBuffer
import com.anpfuel.domain.enterprise.tracking.TrackingBook
import com.anpfuel.domain.enterprise.tracking.TrackingBufferBook
import com.anpfuel.domain.enterprise.tracking.TrackingCause
import com.anpfuel.domain.enterprise.tracking.TrackingCheckpoint
import com.anpfuel.domain.enterprise.tracking.TrackingDevice
import com.anpfuel.domain.enterprise.tracking.TrackingPolicy
import com.anpfuel.domain.enterprise.tracking.TrackingPolicyRead
import com.anpfuel.domain.enterprise.tracking.TrackingSessionState
import com.anpfuel.domain.enterprise.trip.TripAssetLabel
import com.anpfuel.domain.enterprise.trip.TripBook
import java.time.Instant
import java.time.ZoneId
import java.util.UUID
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * EMP-GPS-002/003 on the owned API 36 emulator: the shipped OrganizationTrackingService (manifest entry, foreground start,
 * ongoing notification, one-time visible grant, platform GNSS listener of the emulator's simulated route, notification stop
 * action) over the real tracking service and producers with an in-memory protected queue. The backend path is proven by
 * the data module's EnterpriseTrackingNativeDeviceTest; this class proves what only the app process can show.
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseTrackingServiceDeviceTest {
    private val context: Context get() = InstrumentationRegistry.getInstrumentation().targetContext
    private var scenario: ActivityScenario<ComponentActivity>? = null
    private fun id() = UUID.randomUUID().toString()
    private val partition = EnterprisePartition.of(id(), "https://api.example.invalid", id())
    private val summary = EnterpriseContextSummary(partition, id(), "Transportes Sintéticos", id())
    private val enrollment = EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(partition, summary.membershipId), id(), 1)
    private val identity = EnterpriseLocalIdentity(partition.ownerAccount, partition.origin, id())
    private val records = linkedMapOf<String, EnterpriseLocalRecord>()
    private val entries = linkedMapOf<String, EnterpriseQueueEntry>()
    private val source = object : EnterpriseQueueSource {
        override suspend fun account(): EnterpriseQueueAccountLease = EnterpriseQueueAccountLease.capture(identity, 0, 0, id()) { true }
        override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt =
            account.forEnrollment(enrollment)
    }
    private val hints = EnterpriseTripStartHints()
    private val producers: Map<String, EnterpriseQueueProducer> =
        (EnterpriseTripProducers.all(hints).toList() + EnterpriseTrackingProducers.all().toList()).associateBy { it.descriptor().type }
    /** The protected write in memory: the operation and its producer's projection together or not at all. */
    private val save: suspend (EnterprisePartition, EnterpriseOperation) -> Boolean = { _, operation ->
        synchronized(records) {
            val attempt = runBlocking { source.attempt(source.account(), partition) }
            val changed = records.toMutableMap()
            producers.getValue(operation.type).save(object : EnterpriseQueueTransaction {
                override fun record(id: String) = changed[id]
                override fun put(record: EnterpriseLocalRecord) { changed[record.id] = record }
            }, attempt, operation)
            entries[operation.operationId] = EnterpriseQueueEntry(operation)
            records.clear(); records.putAll(changed); true
        }
    }
    private fun book() = records[EnterpriseTrackingProducers.BOOK_RECORD_ID]?.let { TrackingBook.decode(it.envelope) }
    private fun buffers() = records[EnterpriseTrackingProducers.BUFFER_RECORD_ID]?.let { TrackingBufferBook.decode(it.envelope) }
    private fun put(id: String, bytes: ByteArray) { records[id] = EnterpriseLocalRecord(partition, id, bytes) }
    private val store = object : EnterpriseTrackingStore {
        override suspend fun read(attempt: EnterpriseQueueAttempt): EnterpriseTrackingSnapshot = synchronized(records) {
            val trips = records[EnterpriseTripProducers.BOOK_RECORD_ID]?.let { TripBook.decode(it.envelope) }
            EnterpriseTrackingSnapshot(book(), trips, entries.toMap(), buffers())
        }
        override suspend fun replacePolicy(attempt: EnterpriseQueueAttempt, read: TrackingPolicyRead) = synchronized(records) {
            put(EnterpriseTrackingProducers.BOOK_RECORD_ID, (book() ?: TrackingBook.empty(summary.membershipId, enrollment.enrollmentId)).withPolicy(read).canonicalBytes)
        }
        override suspend fun checkpoint(attempt: EnterpriseQueueAttempt, sessionId: String, checkpoint: TrackingCheckpoint): Boolean = synchronized(records) {
            val current = book()?.takeIf { it.sessionById(sessionId) != null } ?: return false
            put(EnterpriseTrackingProducers.BOOK_RECORD_ID, current.withCheckpoint(sessionId, checkpoint).canonicalBytes); true
        }
        override suspend fun updateBuffer(attempt: EnterpriseQueueAttempt, sessionId: String, change: (SessionBuffer?) -> SessionBuffer?): SessionBuffer? =
            synchronized(records) {
                if (book()?.sessionById(sessionId) == null) return null
                val current = buffers() ?: TrackingBufferBook.empty(summary.membershipId, enrollment.enrollmentId)
                val next = change(current.session(sessionId)) ?: return current.session(sessionId)
                put(EnterpriseTrackingProducers.BUFFER_RECORD_ID, current.with(next).canonicalBytes); next
            }
    }
    private val tripStore = com.anpfuel.application.enterprise.trip.EnterpriseTripStore {
        synchronized(records) {
            val trips = records[EnterpriseTripProducers.BOOK_RECORD_ID]?.let { TripBook.decode(it.envelope) }
            EnterpriseTripSnapshot(trips, trips?.trips?.flatMap { it.operationIds }?.associateWith { entries.getValue(it) } ?: emptyMap())
        }
    }
    private val tracking = EnterpriseTrackingService(source, save, store, { _, at ->
        EnterpriseAccessResult.Ok(TrackingPolicyRead(TrackingPolicy.EXPECTED, 1, "aviso-2026.11", Instant.parse("2026-11-01T00:00:00Z"), at))
    }, { ZoneId.systemDefault() })
    private val trips = EnterpriseTripService(source, save, tripStore, hints, { EnterpriseDeviceClock("boot-1", 1, System.currentTimeMillis()) },
        { ZoneId.systemDefault() })
    private val runtime = TrackingRuntime(TrackingStartGate { android.os.SystemClock.elapsedRealtime() })
    private val notifications get() = context.getSystemService(NotificationManager::class.java)

    @Before fun setUp() {
        val automation = InstrumentationRegistry.getInstrumentation().uiAutomation
        for (permission in listOf(Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION, Manifest.permission.POST_NOTIFICATIONS))
            automation.grantRuntimePermission(context.packageName, permission)
        OrganizationTrackingHost.installed = OrganizationTrackingHost.Installed(runtime, tracking, source)
        scenario = ActivityScenario.launch(ComponentActivity::class.java)
    }

    @After fun tearDown() {
        context.stopService(Intent(context, OrganizationTrackingService::class.java))
        scenario?.close()
        OrganizationTrackingHost.installed = null
    }

    private fun waitFor(what: String, seconds: Int = 30, condition: () -> Boolean) {
        val deadline = System.currentTimeMillis() + seconds * 1000L
        while (!condition()) {
            assertTrue("Timed out waiting for $what", System.currentTimeMillis() < deadline)
            Thread.sleep(250)
        }
    }
    private fun trackingNotification() = notifications.activeNotifications.firstOrNull { it.id == OrganizationTrackingNotifications.NOTIFICATION_ID }

    @Test fun theServiceListensOnlyAfterTheVisibleGrantShowsItsNotificationAndStopsFromIt(): Unit = runBlocking {
        val choice = EnterpriseAssetChoice(partition.organizationId, id(), 1, AssetKind.VEHICLE, UsageBasis.DISTANCE, emptyList(),
            Instant.now(), false)
        trips.start(source.account(), summary, choice, TripAssetLabel("Caminhão sintético", "F-1", ""), TripCapture.Absent("Painel apagado"))
        val trip = trips.trips(source.account(), partition).single().trip.tripId
        tracking.refreshPolicy(source.account(), summary)
        val started = tracking.start(source.account(), summary, trip, TrackingDevice(LocationPermission.PRECISE, OrganizationTrackingNotifications.visible(context)))
        assertEquals(TrackingSessionState.RECORDING, started.state)
        val start = OrganizationTrackingService.startIntent(context, partition, started.sessionId, summary.organizationName)
        // A start intent without the visible screen's grant (a worker, a stale intent) starts nothing and leaves no notification.
        ContextCompat.startForegroundService(context, start)
        Thread.sleep(3_000)
        waitFor("the refused start to stop", 10) { trackingNotification() == null }
        assertNull(runtime.collector)
        // The visible trip screen's tap: grant, then start.
        runtime.gate.arm(partition, started.sessionId)
        ContextCompat.startForegroundService(context, start)
        waitFor("the ongoing notification") { trackingNotification() != null && runtime.collector != null }
        val notification = trackingNotification()!!.notification
        assertEquals(context.getString(R.string.enterprise_tracking_notification_title, summary.organizationName),
            notification.extras.getCharSequence(android.app.Notification.EXTRA_TITLE).toString())
        assertTrue(notification.flags and android.app.Notification.FLAG_ONGOING_EVENT != 0)
        val stopAction = notification.actions.single()
        assertEquals(context.getString(R.string.enterprise_tracking_stop), stopAction.title.toString())
        assertFalse("No coordinate in the notification", notification.extras.toString().contains("-15."))
        // The emulator's simulated route (the driver's geo fixes) reaches the one listener and is recorded in the sealed buffer.
        val collector = runtime.collector!!
        waitFor("recorded emulator fixes", 120) { collector.state.value.recorded >= 2 }
        // A second visible start of the same session adds no listener: the same collector keeps running.
        runtime.gate.arm(partition, started.sessionId)
        ContextCompat.startForegroundService(context, start)
        Thread.sleep(2_000)
        assertSame(collector, runtime.collector)
        assertEquals(1, collector.state.value.let { if (it.active) 1 else 0 })
        // "Parar rastreamento" from the notification: interrupted by the driver, the fixes sealed, the notification gone.
        stopAction.actionIntent.send()
        waitFor("the stop from the notification") { trackingNotification() == null && runtime.collector == null }
        val view = tracking.screen(source.account(), partition).session(trip)!!
        assertEquals(TrackingSessionState.INTERRUPTED, view.state); assertEquals(TrackingCause.STOPPED_BY_DRIVER, view.cause)
        assertEquals(0, view.pendingFixes); assertTrue(view.batches.isNotEmpty())
        // After the stop nothing more is recorded.
        val recorded = collector.state.value.recorded
        Thread.sleep(5_000)
        assertEquals(recorded, collector.state.value.recorded)
    }
}
