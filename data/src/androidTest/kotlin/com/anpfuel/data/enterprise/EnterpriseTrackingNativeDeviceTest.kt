package com.anpfuel.data.enterprise

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.location.LocationManager
import android.os.Looper
import android.os.Process
import android.os.SystemClock
import android.provider.Settings
import androidx.core.content.ContextCompat
import androidx.core.location.LocationCompat
import androidx.core.location.LocationListenerCompat
import androidx.core.location.LocationManagerCompat
import androidx.core.location.LocationRequestCompat
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.fleet.EnterpriseReferenceService
import com.anpfuel.application.enterprise.queue.EnterpriseQueueRegistry
import com.anpfuel.application.enterprise.queue.EnterpriseQueueService
import com.anpfuel.application.enterprise.tracking.EnterpriseTrackingProducers
import com.anpfuel.application.enterprise.tracking.EnterpriseTrackingService
import com.anpfuel.application.enterprise.tracking.TrackingBeginResult
import com.anpfuel.application.enterprise.tracking.TrackingCollector
import com.anpfuel.application.enterprise.tracking.TrackingFixSample
import com.anpfuel.application.enterprise.tracking.TrackingPlatform
import com.anpfuel.application.enterprise.tracking.TrackingSessionView
import com.anpfuel.application.enterprise.tracking.TrackingStartGate
import com.anpfuel.application.enterprise.trip.EnterpriseTripProducers
import com.anpfuel.application.enterprise.trip.EnterpriseTripService
import com.anpfuel.application.enterprise.trip.EnterpriseTripStartHints
import com.anpfuel.application.enterprise.trip.TripCapture
import com.anpfuel.application.enterprise.trip.TripMeterChoice
import com.anpfuel.application.portable.*
import com.anpfuel.data.di.EnterpriseModule
import com.anpfuel.data.enterprise.fleet.EnterpriseReferenceHttpClient
import com.anpfuel.data.enterprise.fleet.ProtectedReferenceCacheStore
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.data.enterprise.tracking.EnterpriseTrackingPolicyHttpGateway
import com.anpfuel.data.enterprise.tracking.ProtectedTrackingStore
import com.anpfuel.data.enterprise.trip.ProtectedTripStore
import com.anpfuel.data.local.auth.AccountHttpApi
import com.anpfuel.data.local.auth.AndroidAnonymousDeviceKeys
import com.anpfuel.data.local.auth.AndroidKeystoreKeys
import com.anpfuel.data.local.auth.KeystoreAccountKey
import com.anpfuel.data.local.auth.KeystoreSessionStore
import com.anpfuel.data.remote.AnonymousProofHttpClient
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.PhotoProofTransport
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.fleet.MyAssets
import com.anpfuel.domain.enterprise.meter.MeterEntryForm
import com.anpfuel.domain.enterprise.meter.MeterKind
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import com.anpfuel.domain.enterprise.sync.SyncLimits
import com.anpfuel.domain.enterprise.tracking.LocationPermission
import com.anpfuel.domain.enterprise.tracking.TrackingBatchOperations
import com.anpfuel.domain.enterprise.tracking.TrackingCause
import com.anpfuel.domain.enterprise.tracking.TrackingDevice
import com.anpfuel.domain.enterprise.tracking.TrackingPolicy
import com.anpfuel.domain.enterprise.tracking.TrackingSessionState
import com.anpfuel.domain.enterprise.trip.TripAssetLabel
import com.anpfuel.domain.enterprise.trip.TripLocalState
import com.anpfuel.domain.portable.PortableAuth
import java.io.Closeable
import java.io.File
import java.net.InetAddress
import java.security.KeyStore
import java.security.cert.CertificateFactory
import java.time.ZoneId
import java.util.UUID
import java.util.concurrent.atomic.AtomicInteger
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManagerFactory
import javax.net.ssl.X509TrustManager
import kotlinx.coroutines.runBlocking
import okhttp3.Dns
import okhttp3.OkHttpClient
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Opt-in two-process EMP-GPS-002/003 acceptance on the owned API 36 emulator against the private ordinary PostGIS/HTTPS
 * tracking fixture (orgsync_tracking_native_fixture_test.go), with the product's real protected store, queue, producers,
 * signer, HTTP clients, tracking service and collector. Positions are the emulator's own GNSS fixes set by the driver with
 * `adb emu geo fix` (a simulated route, never a real location); the test-only listener below feeds them to the real
 * collector exactly as the shipped service's platform does (the shipped service itself is proven by the app device test).
 * Stage 1 reads the policy, starts the trip and tracking, records, stops from the "notification" path, resumes and records
 * again; the driver then force-stops the process. Stage 2 is a new process: the interruption is found, the waiting fixes are
 * sealed, the trip is finished, and every operation and batch is accepted by the real intake.
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseTrackingNativeDeviceTest {
    private val context: Context get() = InstrumentationRegistry.getInstrumentation().targetContext

    @Test
    fun stage1PolicyTripTrackingStopResumeThenTheProcessDies(): Unit = runBlocking {
        val fixture = readFixture(File(context.noBackupFilesDir, FIXTURE))
        val graph = Graph(context, fixture, seed = true)
        try {
            assertEquals("Unexpected previous native tracking stage", 0, graph.preferences.getInt(STAGE_NUMBER, 0))
            assertEquals("Location permission must be granted by the driver", PackageManager.PERMISSION_GRANTED,
                ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION))
            graph.establishCustody()
            // The vehicle from the signed reference pull and the policy in force from the company read (B-BR-EMP-495).
            val cache = requireNotNull(graph.references.refresh(graph.source.deliveryAccount(), fixture.partition).cache)
            val policy = requireNotNull(graph.tracking.refreshPolicy(graph.source.deliveryAccount(), fixture.context))
            assertEquals(TrackingPolicy.EXPECTED, policy.policy); assertEquals("aviso-2026.11", policy.noticeVersion)
            val snapshot = MyAssets.present(cache, graph.clock.read())
            val choice = requireNotNull(MyAssets.choose(fixture.organization, snapshot, fixture.asset, warningAccepted = true))
            val asset = snapshot.assets.single { it.asset.id == fixture.asset }.asset
            val meter = choice.meters.single()
            graph.trips.start(graph.source.account(), fixture.context, choice, TripAssetLabel(asset.name, asset.fleetNumber, asset.plate),
                TripCapture.Reading(TripMeterChoice(meter.id, meter.segmentId, MeterKind.ODOMETER, meter.modulus), MeterEntryForm.KM, "184320"))
            val trip = graph.trips.trips(graph.source.account(), fixture.partition).single().trip.tripId
            // The driver's tap: start with the permission observed now.
            val started = graph.tracking.start(graph.source.account(), fixture.context, trip, TrackingDevice(graph.platform.permission(), true))
            assertEquals(TrackingSessionState.RECORDING, started.state)
            assertEquals(LocationPermission.PRECISE, started.precision)
            val session = started.sessionId
            graph.preferences.edit().putString(TRIP, trip).putString(SESSION, session).commit()
            // Without the visible grant nothing listens.
            val first = graph.collector()
            assertEquals(TrackingBeginResult.NO_GRANT, first.begin(fixture.partition, session))
            graph.gate.arm(fixture.partition, session)
            assertEquals(TrackingBeginResult.STARTED, first.begin(fixture.partition, session))
            graph.recordUntil(first, 3)
            assertTrue("The checkpoint is written while recording", first.tick())
            // "Parar rastreamento" from the notification: the interruption is written and the recorded fixes leave in a batch.
            assertTrue(first.stopByDriver())
            val stopped = graph.view(trip)
            assertEquals(TrackingSessionState.INTERRUPTED, stopped.state); assertEquals(TrackingCause.STOPPED_BY_DRIVER, stopped.cause)
            assertEquals(0, stopped.pendingFixes); assertEquals(1, stopped.batches.size)
            assertTrue(graph.platform.attached == 0)
            // "Retomar rastreamento": a resume on the visible screen, a new grant, recording again.
            assertEquals(TrackingSessionState.RECORDING, graph.tracking.start(graph.source.account(), fixture.context, trip,
                TrackingDevice(graph.platform.permission(), true)).state)
            val second = graph.collector()
            graph.gate.arm(fixture.partition, session)
            assertEquals(TrackingBeginResult.STARTED, second.begin(fixture.partition, session))
            graph.recordUntil(second, 2)
            assertTrue(graph.view(trip).pendingFixes >= 2)
            graph.mark(1)
            // The process is left recording; the driver force-stops it (process death, BR-GPS-011).
        } finally { graph.close() }
    }

    @Test
    fun stage2AfterProcessDeathTheInterruptionIsFoundAndEverythingIsAccepted(): Unit = runBlocking {
        val fixture = readFixture(File(context.noBackupFilesDir, FIXTURE))
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(1)
            val trip = requireNotNull(graph.preferences.getString(TRIP, null))
            val before = graph.view(trip)
            assertEquals("The session was recording when the process died", TrackingSessionState.RECORDING, before.state)
            assertTrue("The buffer survived the process death", before.pendingFixes >= 2)
            // The next opening of the trip screen: no collector runs in this process.
            assertEquals(1, graph.tracking.recover(graph.source.account(), fixture.partition, { false }, graph.platform.bootId()))
            val interrupted = graph.view(trip)
            assertEquals(TrackingSessionState.INTERRUPTED, interrupted.state); assertEquals(TrackingCause.PROCESS_STOPPED, interrupted.cause)
            assertEquals(0, interrupted.pendingFixes); assertEquals(2, interrupted.batches.size)
            assertNull("Nothing resumes by itself", graph.tracking.collecting(graph.source.account(), fixture.partition, interrupted.sessionId))
            // The driver finishes the trip; the session ends at the finish's own instant.
            val view = graph.trips.trips(graph.source.account(), fixture.partition).single()
            val departure = view.start.fact as com.anpfuel.domain.enterprise.trip.TripOperationFact.Start
            val evidence = departure.evidence as com.anpfuel.domain.enterprise.trip.TripEvidence.Reading
            graph.trips.finish(graph.source.account(), fixture.partition, trip,
                TripCapture.Reading(TripMeterChoice(evidence.meterId, evidence.segmentId, MeterKind.ODOMETER, null), MeterEntryForm.KM, "184330"))
            assertEquals(TripLocalState.FINISHED, graph.trips.trips(graph.source.account(), fixture.partition).single().trip.state)
            assertEquals(TrackingSessionState.ENDED, requireNotNull(graph.tracking.follow(graph.source.account(), fixture.partition, trip)).state)
            var accepted: TrackingSessionView? = null
            val diagnostics = StringBuilder()
            for (attempt in 1..20) {
                val result = try { graph.delivery.dispatch(graph.source.deliveryAccount(), fixture.partition) }
                    catch (failure: Exception) { diagnostics.append("dispatch ").append(failure.javaClass.simpleName).append("; "); null }
                val current = graph.view(trip)
                result?.let { diagnostics.append("attempted=${it.attempted} completed=${it.completed} retryable=${it.retryable} wait=${it.retryAfterMillis} ") }
                diagnostics.append("http=${graph.statuses} ").append((current.records + current.batches).joinToString(",") { "${it.type}/${it.sync}/${it.code}" }).append("; ")
                if ((current.records + current.batches).all { it.sync == EnterpriseSyncStatus.ACCEPTED }) { accepted = current; break }
                Thread.sleep(3_000)
            }
            val done = requireNotNull(accepted) { "Tracking was not accepted after delivery: $diagnostics" }
            assertEquals(listOf(TrackingSessionState.RECORDING, TrackingSessionState.INTERRUPTED, TrackingSessionState.RECORDING,
                TrackingSessionState.INTERRUPTED, TrackingSessionState.ENDED).size, done.records.size)
            assertEquals(2, done.batches.size)
            val sent = graph.operationsSent.get()
            graph.delivery.dispatch(graph.source.deliveryAccount(), fixture.partition)
            assertEquals("An accepted record was sent again", sent, graph.operationsSent.get())
            // No coordinate, session or trip identifier is readable outside encryption.
            val batch = TrackingBatchOperations.read(graph.store.read(graph.source.attempt(graph.source.account(), fixture.partition))
                .entries.getValue(done.batches.first().operationId).operation)
            graph.assertProtectedSurfaces(listOf(trip, done.sessionId, batch.fixes.first().latE7.toString(), TrackingBatchOperations.TYPE, "tracking-buffer"))
            graph.cleanup()
        } finally { graph.close() }
    }

    private class SourceFixture(val origin: String, val certificate: String, val session: PortableAuth.Session, val organization: String,
        val member: String, val employee: String, val asset: String) {
        val partition: EnterprisePartition get() = EnterprisePartition.of(session.accountId, origin, organization)
        val context: EnterpriseContextSummary get() = EnterpriseContextSummary(partition, member, "Synthetic tracking", employee)
        override fun toString(): String = "TrackingSourceFixture(redacted)"
    }

    /** The emulator's GNSS fixes through one test-only listener, never shipped (the guard checks shipped sources only). */
    private class EmulatorPlatform(private val context: Context) : TrackingPlatform {
        private val manager = context.getSystemService(LocationManager::class.java)
        private var listener: LocationListenerCompat? = null
        var attached = 0
        override fun permission(): LocationPermission =
            if (ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED) LocationPermission.PRECISE
            else LocationPermission.NONE
        override fun notificationVisible(): Boolean = true
        override fun bootId(): String = "boot-" + Settings.Global.getInt(context.contentResolver, Settings.Global.BOOT_COUNT, 0)
        override fun attach(precision: LocationPermission, sink: (TrackingFixSample) -> Unit): Boolean {
            val active = LocationListenerCompat { l -> sink(TrackingFixSample(l.latitude, l.longitude, if (l.hasAccuracy()) l.accuracy else null,
                if (l.hasSpeed()) l.speed else null, if (l.hasBearing()) l.bearing else null, l.time, l.elapsedRealtimeNanos / 1_000_000, LocationCompat.isMock(l))) }
            listener = active; attached++
            InstrumentationRegistry.getInstrumentation().runOnMainSync {
                LocationManagerCompat.requestLocationUpdates(manager, LocationManager.GPS_PROVIDER,
                    LocationRequestCompat.Builder(1_000L).setQuality(LocationRequestCompat.QUALITY_HIGH_ACCURACY).build(), active, Looper.getMainLooper())
            }
            return true
        }
        override fun detach() {
            val active = listener ?: return
            listener = null; attached--
            InstrumentationRegistry.getInstrumentation().runOnMainSync { LocationManagerCompat.removeUpdates(manager, active) }
        }
        override fun wallMillis(): Long = System.currentTimeMillis()
        override fun elapsedMillis(): Long = SystemClock.elapsedRealtime()
        override fun batteryLow(): Boolean = false
        override fun providerEnabled(precision: LocationPermission): Boolean = manager.isProviderEnabled(LocationManager.GPS_PROVIDER)
    }

    private class Graph(private val context: Context, private val fixture: SourceFixture, seed: Boolean) : Closeable {
        val preferences = context.getSharedPreferences("enterprise-native-tracking", Context.MODE_PRIVATE)
        private val fence = EnterpriseSessionFence()
        private val platformKeys = AndroidKeystoreKeys()
        private val sessions = EnterpriseFencedSessionStore(KeystoreSessionStore(preferences, platformKeys, SESSION_ALIAS), fence)
        private val accountKeys = KeystoreAccountKey(preferences, platformKeys, BACKUP_ALIAS)
        init {
            if (seed) {
                assertNull("Unexpected existing native tracking session", sessions.load())
                sessions.save(fixture.session)
            }
            val current = sessions.load() ?: throw AssertionError("Retained native tracking session is unavailable")
            assertTrue("Retained source changed ownership", current.accountId == fixture.session.accountId && current.familyId == fixture.session.familyId)
        }
        val operationsSent = AtomicInteger()
        val statuses: MutableList<Int> = java.util.Collections.synchronizedList(mutableListOf())
        private val http = fixtureHttp(fixture.certificate).newBuilder().addInterceptor { chain ->
            chain.proceed(chain.request()).also {
                if (chain.request().url.encodedPath.endsWith("/sync/operations")) { operationsSent.incrementAndGet(); statuses += it.code }
            }
        }.build()
        private val auth = AuthFlow(AuthPorts(sessions, accountKeys, AuthWallClock { System.currentTimeMillis() / 1000 },
            PortableNonceSource { UUID.randomUUID().toString() }, AccountHttpApi(http, fixture.origin)))
        private val deviceKeys = AndroidAnonymousDeviceKeys(auth, ApiEnvironment.PREVIEW)
        private val proof = PhotoProofTransport(deviceKeys, AnonymousProofHttpClient(http, fixture.origin), auth, fixture.origin, http)
        private val signer = EnterpriseIdentityV1ContactSigner(proof)
        private val contact = EnterpriseDeviceContactClient(http, fixture.origin, sessions, auth, fence, signer)
        private val files = EnterpriseModule.vaultFiles(context)
        private val keys = EnterpriseModule.vaultKeys()
        private val erasure = EnterpriseVaultErasure(files, keys, fence)
        val clock = AndroidEnterpriseDeviceClock(context)
        private val persistence = ProtectedEnterprisePersistence(files, keys, fence, erasure,
            { sessions.load()?.let { EnterpriseLocalIdentity(it.accountId, fixture.origin, it.familyId) } }, clock::read, contact)
        val source = EnterpriseOperationSessionSource(fixture.origin, sessions, auth, fence, persistence::protectedEnrollment)
        val references = EnterpriseReferenceService(source, ProtectedReferenceCacheStore(persistence),
            EnterpriseReferenceHttpClient(http, source, signer), clock::read)
        private val queue = ProtectedEnterpriseQueue(persistence)
        private val hints = EnterpriseTripStartHints()
        val delivery = EnterpriseQueueService(source, queue, EnterpriseOperationHttpClient(http, source, signer, SyncLimits.DEFAULT),
            EnterpriseQueueRegistry(*EnterpriseTripProducers.all(hints), *EnterpriseTrackingProducers.all()), clock::read, entropy = { 0L })
        val trips = EnterpriseTripService(source, delivery::save, ProtectedTripStore(persistence), hints, clock::read, { ZoneId.systemDefault() })
        val store = ProtectedTrackingStore(persistence)
        val tracking = EnterpriseTrackingService(source, delivery::save, store,
            EnterpriseTrackingPolicyHttpGateway(CompanyAccessClient(http, fixture.origin, sessions, auth, fence)), { ZoneId.systemDefault() })
        val gate = TrackingStartGate { SystemClock.elapsedRealtime() }
        val platform = EmulatorPlatform(context)
        fun collector() = TrackingCollector(tracking, source, gate, platform) { account, partition -> delivery.dispatch(account, partition) }

        suspend fun view(trip: String): TrackingSessionView = requireNotNull(tracking.screen(source.account(), fixture.partition).session(trip))

        /** Records until [count] fixes are recorded from the driver's simulated route (at most 120 s). */
        suspend fun recordUntil(collector: TrackingCollector, count: Long) {
            val deadline = SystemClock.elapsedRealtime() + 120_000
            while (collector.state.value.recorded < count) {
                assertTrue("No emulator fix was recorded: accepted=${collector.state.value.accepted} recorded=${collector.state.value.recorded}",
                    SystemClock.elapsedRealtime() < deadline)
                Thread.sleep(1_000)
                collector.drain()
            }
        }

        fun mark(stage: Int) = assertTrue("Stage marker must be durable before process termination",
            preferences.edit().putInt(STAGE, Process.myPid()).putInt(STAGE_NUMBER, stage).commit())
        fun requireStage(previous: Int) {
            val pid = preferences.getInt(STAGE, 0)
            assertTrue("Stage $previous must run in an earlier process", pid > 0 && pid != Process.myPid() && preferences.getInt(STAGE_NUMBER, 0) == previous)
        }

        suspend fun establishCustody() {
            val local = EnterpriseLocalContext({ sessions.load()?.accountId }, { fixture.origin }, persistence, { fence.epoch })
            fun binding() = sessions.load()?.let { EnterpriseAccessBinding(it.accountId, fixture.origin, it.familyId, fence.epoch) }
            val before = binding() ?: throw AssertionError("Current native proof is unavailable")
            val owner = local.reserveContact(local.acquireOwner(), before, fence, ::binding)
            val handoff = persistence.confirm(EnterpriseDeviceTarget(fixture.partition, fixture.member), before, owner)
            local.select(local.consumeContact(owner, handoff, binding()), fixture.partition)
        }

        /** No trip, session, coordinate, operation type or record format appears outside encryption. */
        fun assertProtectedSurfaces(secrets: List<String>) {
            val token = EnterpriseVaultCodec.token(fixture.partition)
            for (generation in files.generations(token)) for (descriptor in listOf(false, true)) {
                val text = String(files.readGeneration(token, generation, descriptor) ?: continue, Charsets.ISO_8859_1)
                for (secret in secrets + fixture.session.accountId + fixture.session.accessToken)
                    assertFalse("A tracking value appeared outside encryption", text.contains(secret))
            }
        }

        fun cleanup() {
            assertTrue("Native tracking cleanup was incomplete", erasure.partitionRevoked(fixture.partition))
            val identityAlias = AndroidAnonymousDeviceKeys.aliasFor(AuthFlow.AuthState.Active(fixture.session), fixture.origin)
            KeyStore.getInstance("AndroidKeyStore").apply { load(null); deleteEntry(identityAlias); deleteEntry(SESSION_ALIAS); deleteEntry(BACKUP_ALIAS) }
            assertTrue(preferences.edit().clear().commit())
            val file = File(context.noBackupFilesDir, FIXTURE)
            assertTrue("Private native proof cleanup failed", !file.exists() || file.delete())
        }

        override fun close() {
            platform.detach()
            http.connectionPool.evictAll()
            http.dispatcher.executorService.shutdown()
        }
    }

    companion object {
        private const val FIXTURE = "enterprise-native-tracking-fixture.json"
        private const val STAGE = "tracking_stage_process"
        private const val STAGE_NUMBER = "tracking_stage_number"
        private const val TRIP = "tracking_trip"
        private const val SESSION = "tracking_session"
        private const val SESSION_ALIAS = "enterprise_native_tracking_session"
        private const val BACKUP_ALIAS = "enterprise_native_tracking_backup"

        private fun readFixture(file: File): SourceFixture = try {
            require(file.isFile && file.length() in 1..32_768)
            val value = JSONObject(file.readText())
            val origin = value.getString("origin").also { require(it == ApiEnvironment.PREVIEW.origin) }
            fun id(name: String) = EnterprisePartition.requireId(value.getString(name))
            fun token(name: String) = value.getString(name).also { require(it.length in 1..512 && it.all { c -> c.code in 33..126 }) }
            val session = PortableAuth.Session(id("family_id"), id("account_id"), token("access_token"), token("refresh_token"),
                value.getLong("access_expires_at"), value.getLong("refresh_expires_at"))
            val certificate = value.getString("certificate_pem").also { require(it.length in 1..8192) }
            SourceFixture(origin, certificate, session, id("org_id"), id("membership_id"), id("employee_id"), id("asset_id"))
        } catch (_: Exception) { throw AssertionError("Disposable tracking fixture is invalid") }

        private fun fixtureHttp(pem: String): OkHttpClient = try {
            val certificate = CertificateFactory.getInstance("X.509").generateCertificate(pem.byteInputStream())
            val anchors = KeyStore.getInstance(KeyStore.getDefaultType()).apply { load(null); setCertificateEntry("disposable-tracking", certificate) }
            val trust = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply { init(anchors) }
                .trustManagers.single() as X509TrustManager
            val tls = SSLContext.getInstance("TLS").apply { init(null, arrayOf(trust), null) }
            OkHttpClient.Builder().sslSocketFactory(tls.socketFactory, trust).dns(object : Dns {
                override fun lookup(hostname: String): List<InetAddress> {
                    check(hostname == "api.anpfuel.example.invalid") { "Native tracking test refused an unrelated host" }
                    return listOf(InetAddress.getByAddress(byteArrayOf(127, 0, 0, 1)))
                }
            }).followRedirects(false).followSslRedirects(false).retryOnConnectionFailure(false).build()
        } catch (_: Exception) { throw AssertionError("Disposable tracking trust anchor is invalid") }
    }
}
