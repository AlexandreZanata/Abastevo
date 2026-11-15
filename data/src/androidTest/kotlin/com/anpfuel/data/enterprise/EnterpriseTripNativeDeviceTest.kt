package com.anpfuel.data.enterprise

import android.content.Context
import android.os.Process
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.fleet.EnterpriseReferenceService
import com.anpfuel.application.enterprise.queue.EnterpriseQueueRegistry
import com.anpfuel.application.enterprise.queue.EnterpriseQueueService
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
import com.anpfuel.domain.enterprise.trip.TripAssetLabel
import com.anpfuel.domain.enterprise.trip.TripLocalState
import com.anpfuel.domain.enterprise.trip.TripLogbookType
import com.anpfuel.domain.enterprise.trip.TripOperations
import com.anpfuel.domain.enterprise.trip.TripReviewLine
import com.anpfuel.domain.enterprise.trip.TripView
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
 * Opt-in five-process EMP-TRIP-005 acceptance against the private ordinary PostGIS/HTTPS trip fixture
 * (orgsync_trip_native_fixture_test.go), with the product's real protected store, queue, producers, signer and
 * HTTP clients. Stage 1 is online (custody and the reference pull for the picker); the driver then force-stops
 * this target and removes the route; stages 2 and 3 record the trip offline across a process death; the route
 * returns for stage 4 (delivery, exactly once) and stage 5 (a start held behind another member's start).
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseTripNativeDeviceTest {
    private val context: Context get() = InstrumentationRegistry.getInstrumentation().targetContext

    @Test
    fun stage1OnlineCustodyAndReferencePull(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = true)
        try {
            assertEquals("Unexpected previous native trip stage", 0, graph.preferences.getInt(STAGE_NUMBER, 0))
            graph.establishCustody()
            val (a, _) = fixture.organizations
            val cache = requireNotNull(graph.references.refresh(graph.source.deliveryAccount(), a.partition(fixture)).cache)
            val vehicle = cache.assets.single { it.id == a.asset }
            assertEquals(listOf(a.meter!!), vehicle.meters.map { it.id })
            assertTrue(graph.trips(a).isEmpty())
            graph.mark(1)
        } finally { graph.close() }
    }

    @Test
    fun stage2OfflineStartAndStopAreDurableBeforeAnyDelivery(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(1)
            val (a, _) = fixture.organizations
            // The picker's choice from the protected reference cache, with no network.
            val cached = requireNotNull(graph.references.cached(graph.source.account(), a.partition(fixture)))
            val snapshot = MyAssets.present(cached, graph.clock.read())
            val choice = requireNotNull(MyAssets.choose(a.id, snapshot, a.asset, warningAccepted = false))
            val asset = snapshot.assets.single { it.asset.id == a.asset }.asset
            val meter = choice.meters.single()
            val departure = TripCapture.Reading(TripMeterChoice(meter.id, meter.segmentId, MeterKind.ODOMETER, meter.modulus), MeterEntryForm.KM, "184320")
            graph.service.start(graph.source.account(), a.context(fixture), choice, TripAssetLabel(asset.name, asset.fleetNumber, asset.plate), departure)
            val trip = graph.trips(a).single()
            graph.service.log(graph.source.account(), a.partition(fixture), trip.trip.tripId, TripLogbookType.STOP, null)
            val saved = graph.trips(a).single()
            assertEquals(2, saved.trip.operationIds.size)
            assertTrue("Nothing is accepted before a receipt", (listOf(saved.start) + saved.events).all { it.sync == EnterpriseSyncStatus.SAVED_ON_DEVICE })
            // Without a route, a delivery attempt changes nothing and sends nothing.
            graph.delivery.dispatch(graph.source.account(), a.partition(fixture))
            assertEquals(0, graph.operationsSent.get())
            assertTrue(graph.trips(a).single().let { (listOf(it.start) + it.events).all { r -> r.sync == EnterpriseSyncStatus.SAVED_ON_DEVICE } })
            graph.preferences.edit().putString(TRIP, trip.trip.tripId).commit()
            graph.mark(2)
        } finally { graph.close() }
    }

    @Test
    fun stage3OfflineAfterProcessDeathFinishesAndMarksReady(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(2)
            val (a, b) = fixture.organizations
            val tripId = requireNotNull(graph.preferences.getString(TRIP, null))
            val restored = graph.trips(a).single()
            assertEquals("Process death lost the trip", tripId, restored.trip.tripId)
            assertEquals(TripLogbookType.STOP, (restored.events.single().fact as com.anpfuel.domain.enterprise.trip.TripOperationFact.Event).type)
            val account = graph.source.account()
            graph.service.log(account, a.partition(fixture), tripId, TripLogbookType.NOTE, "Entrega sintética conferida")
            val meter = graph.departureMeter(restored)
            graph.service.finish(graph.source.account(), a.partition(fixture), tripId,
                TripCapture.Reading(meter, MeterEntryForm.KM, "184610"))
            graph.service.markReady(graph.source.account(), a.partition(fixture), tripId)
            val ready = graph.trips(a).single()
            assertEquals(TripLocalState.FINISHED, ready.trip.state)
            assertEquals(TripReviewLine.READY_TO_SEND, ready.review)
            assertTrue((listOf(ready.start, ready.finish!!, ready.submission!!) + ready.events).all { it.sync == EnterpriseSyncStatus.SAVED_ON_DEVICE })
            // BR-OFFL-003: organization B of the same account holds nothing of A's trip, and A's work stays sealed to A.
            assertTrue(graph.trips(b).isEmpty())
            assertTrue(graph.entries(a).values.all { it.operation.binding.organizationId == a.id && it.operation.membershipId == a.member })
            graph.assertProtectedSurfaces(listOf(tripId, "Entrega sintética conferida", "184610000", TripOperations.START, "trip-book"))
            assertEquals(0, graph.operationsSent.get())
            graph.mark(3)
        } finally { graph.close() }
    }

    @Test
    fun stage4ReconnectionDeliversEachRecordExactlyOnce(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(3)
            val (a, _) = fixture.organizations
            val tripId = requireNotNull(graph.preferences.getString(TRIP, null))
            var accepted: TripView? = null
            val diagnostics = StringBuilder()
            for (attempt in 1..20) {
                val result = try { graph.delivery.dispatch(graph.source.deliveryAccount(), a.partition(fixture)) }
                    catch (failure: Exception) { diagnostics.append("dispatch ").append(failure.javaClass.simpleName).append("; "); null }
                val view = graph.trips(a).single()
                result?.let { diagnostics.append("attempted=${it.attempted} completed=${it.completed} retryable=${it.retryable} attention=${it.attention} wait=${it.retryAfterMillis} ") }
                diagnostics.append("sent=${graph.operationsSent.get()} http=${graph.statuses} ").append((listOf(view.start) + view.events + listOfNotNull(view.finish, view.submission))
                    .joinToString(",") { "${it.sync}/${it.code}" }).append("; ")
                if (view.review == TripReviewLine.AWAITING_APPROVAL) { accepted = view; break }
                Thread.sleep(3_000)
            }
            val view = requireNotNull(accepted) { "The trip was not accepted after reconnection: $diagnostics" }
            assertEquals(tripId, view.trip.tripId)
            assertTrue((listOf(view.start, view.finish!!, view.submission!!) + view.events).all { it.sync == EnterpriseSyncStatus.ACCEPTED })
            val sent = graph.operationsSent.get()
            graph.delivery.dispatch(graph.source.deliveryAccount(), a.partition(fixture))
            assertEquals("An accepted record was sent again", sent, graph.operationsSent.get())
            // A second trip on the same vehicle is allowed locally: the first one is finished on this device.
            val first = graph.trips(a).single()
            val meter = graph.departureMeter(first)
            val cached = requireNotNull(graph.references.cached(graph.source.account(), a.partition(fixture)))
            val choice = requireNotNull(MyAssets.choose(a.id, MyAssets.present(cached, graph.clock.read()), a.asset, warningAccepted = true))
            graph.service.start(graph.source.account(), a.context(fixture), choice, first.trip.label,
                TripCapture.Reading(meter, MeterEntryForm.KM, "184700"))
            val second = graph.trips(a).first { it.trip.tripId != tripId }
            graph.service.log(graph.source.account(), a.partition(fixture), second.trip.tripId, TripLogbookType.STOP, "Pátio")
            graph.preferences.edit().putString(SECOND_TRIP, second.trip.tripId).commit()
            graph.mark(4)
        } finally { graph.close() }
    }

    @Test
    fun stage5AStartAfterAnotherMembersStartIsHeldForReview(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(4)
            assertTrue("The second member's start must precede this delivery", File(context.noBackupFilesDir, "enterprise-native-trip-second-started").exists())
            val (a, _) = fixture.organizations
            val tripId = requireNotNull(graph.preferences.getString(TRIP, null))
            val secondId = requireNotNull(graph.preferences.getString(SECOND_TRIP, null))
            graph.delivery.dispatch(graph.source.deliveryAccount(), a.partition(fixture))
            val held = graph.trips(a).single { it.trip.tripId == secondId }
            assertEquals(EnterpriseSyncStatus.NEEDS_REVIEW, held.start.sync)
            assertEquals("ASSET_BUSY", held.start.code)
            assertTrue("The trip says its other records wait for the start's review", held.waitingForStartReview)
            assertEquals(EnterpriseSyncStatus.SAVED_ON_DEVICE, held.events.single().sync)
            assertEquals(TripLocalState.IN_PROGRESS, held.trip.state)
            val first = graph.trips(a).single { it.trip.tripId == tripId }
            assertEquals(TripReviewLine.AWAITING_APPROVAL, first.review)
        } finally {
            graph.cleanup()
            graph.close()
        }
    }

    private fun fixtureFile() = File(context.noBackupFilesDir, "enterprise-native-trip-fixture.json")

    private class Organization(val id: String, val member: String, val employee: String, val asset: String, val meter: String?, val segment: String?) {
        fun partition(fixture: SourceFixture) = EnterprisePartition.of(fixture.session.accountId, fixture.origin, id)
        fun context(fixture: SourceFixture) = EnterpriseContextSummary(partition(fixture), member, "Synthetic trips", employee)
        override fun toString(): String = "TripFixtureOrganization(redacted)"
    }
    private class SourceFixture(val origin: String, val certificate: String, val session: PortableAuth.Session, val organizations: List<Organization>) {
        override fun toString(): String = "TripSourceFixture(redacted)"
    }

    private class Graph(private val context: Context, private val fixture: SourceFixture, seed: Boolean) : Closeable {
        val preferences = context.getSharedPreferences("enterprise-native-trip", Context.MODE_PRIVATE)
        private val fence = EnterpriseSessionFence()
        private val platformKeys = AndroidKeystoreKeys()
        private val sessions = EnterpriseFencedSessionStore(KeystoreSessionStore(preferences, platformKeys, SESSION_ALIAS), fence)
        private val accountKeys = KeystoreAccountKey(preferences, platformKeys, BACKUP_ALIAS)
        init {
            if (seed) {
                assertNull("Unexpected existing native trip session", sessions.load())
                sessions.save(fixture.session)
            }
            val current = sessions.load() ?: throw AssertionError("Retained native trip session is unavailable")
            // The retained session is the fixture's own family: a refresh never meets another family (FLEET-013 incident).
            assertTrue("Retained source changed ownership", current.accountId == fixture.session.accountId && current.familyId == fixture.session.familyId)
        }
        /** Intake answers actually received from the source (a refused connection is never counted). */
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
            EnterpriseQueueRegistry(*EnterpriseTripProducers.all(hints)), clock::read, entropy = { 0L })
        private val store = ProtectedTripStore(persistence)
        val service = EnterpriseTripService(source, delivery::save, store, hints, clock::read, { ZoneId.systemDefault() })

        suspend fun trips(organization: Organization): List<TripView> = service.trips(source.account(), organization.partition(fixture))
        suspend fun entries(organization: Organization) = store.read(source.attempt(source.account(), organization.partition(fixture))).entries
        fun departureMeter(view: TripView): TripMeterChoice {
            val evidence = (view.start.fact as com.anpfuel.domain.enterprise.trip.TripOperationFact.Start).evidence
                as com.anpfuel.domain.enterprise.trip.TripEvidence.Reading
            return TripMeterChoice(evidence.meterId, evidence.segmentId, evidence.value.kind, null)
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
            for (organization in fixture.organizations) {
                val before = binding() ?: throw AssertionError("Current native proof is unavailable")
                val owner = local.reserveContact(local.acquireOwner(), before, fence, ::binding)
                val handoff = persistence.confirm(EnterpriseDeviceTarget(organization.partition(fixture), organization.member), before, owner)
                local.select(local.consumeContact(owner, handoff, binding()), organization.partition(fixture))
            }
            assertTrue("Actual identity-v1 signing key was not created", deviceKeys.hasKey())
        }

        /** No trip fact, label or credential appears outside encryption. */
        fun assertProtectedSurfaces(secrets: List<String>) {
            for (organization in fixture.organizations) {
                val token = EnterpriseVaultCodec.token(organization.partition(fixture))
                for (generation in files.generations(token)) for (descriptor in listOf(false, true)) {
                    val text = String(files.readGeneration(token, generation, descriptor) ?: continue, Charsets.ISO_8859_1)
                    for (secret in secrets + fixture.session.accountId + fixture.session.accessToken)
                        assertFalse("A trip value appeared outside encryption", text.contains(secret))
                }
            }
        }

        fun cleanup() {
            fixture.organizations.forEach { assertTrue("Native trip cleanup was incomplete", erasure.partitionRevoked(it.partition(fixture))) }
            val identityAlias = AndroidAnonymousDeviceKeys.aliasFor(AuthFlow.AuthState.Active(fixture.session), fixture.origin)
            KeyStore.getInstance("AndroidKeyStore").apply {
                load(null); deleteEntry(identityAlias); deleteEntry(SESSION_ALIAS); deleteEntry(BACKUP_ALIAS)
            }
            assertTrue(preferences.edit().clear().commit())
            for (name in listOf("enterprise-native-trip-fixture.json", "enterprise-native-trip-second-started")) {
                val file = File(context.noBackupFilesDir, name)
                assertTrue("Private native proof cleanup failed", !file.exists() || file.delete())
                assertFalse("Private native proof survived cleanup", file.exists())
            }
        }

        override fun close() {
            http.connectionPool.evictAll()
            http.dispatcher.executorService.shutdown()
        }
    }

    companion object {
        private const val STAGE = "trip_stage_process"
        private const val STAGE_NUMBER = "trip_stage_number"
        private const val TRIP = "trip_first"
        private const val SECOND_TRIP = "trip_second"
        private const val SESSION_ALIAS = "enterprise_native_trip_session"
        private const val BACKUP_ALIAS = "enterprise_native_trip_backup"

        private fun readFixture(file: File): SourceFixture = try {
            require(file.isFile && file.length() in 1..32_768)
            val value = JSONObject(file.readText())
            val origin = value.getString("origin").also { require(it == ApiEnvironment.PREVIEW.origin) }
            val account = EnterprisePartition.requireId(value.getString("account_id"))
            val family = EnterprisePartition.requireId(value.getString("family_id"))
            fun token(name: String) = value.getString(name).also { require(it.length in 1..512 && it.all { c -> c.code in 33..126 }) }
            val session = PortableAuth.Session(family, account, token("access_token"), token("refresh_token"),
                value.getLong("access_expires_at"), value.getLong("refresh_expires_at"))
            val certificate = value.getString("certificate_pem").also { require(it.length in 1..8192) }
            val organizations = value.getJSONArray("organizations").also { require(it.length() == 2) }.let { rows ->
                (0 until rows.length()).map { index -> rows.getJSONObject(index).let {
                    fun optional(name: String) = if (it.has(name)) EnterprisePartition.requireId(it.getString(name)) else null
                    Organization(EnterprisePartition.requireId(it.getString("org_id")), EnterprisePartition.requireId(it.getString("membership_id")),
                        EnterprisePartition.requireId(it.getString("employee_id")), EnterprisePartition.requireId(it.getString("asset_id")),
                        optional("meter_id"), optional("segment_id"))
                } }
            }
            require(organizations.map { it.id }.distinct().size == 2 && organizations[0].meter != null)
            SourceFixture(origin, certificate, session, organizations)
        } catch (_: Exception) { throw AssertionError("Disposable trip fixture is invalid") }

        private fun fixtureHttp(pem: String): OkHttpClient = try {
            val certificate = CertificateFactory.getInstance("X.509").generateCertificate(pem.byteInputStream())
            val anchors = KeyStore.getInstance(KeyStore.getDefaultType()).apply { load(null); setCertificateEntry("disposable-trip", certificate) }
            val trust = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply { init(anchors) }
                .trustManagers.single() as X509TrustManager
            val tls = SSLContext.getInstance("TLS").apply { init(null, arrayOf(trust), null) }
            OkHttpClient.Builder().sslSocketFactory(tls.socketFactory, trust).dns(object : Dns {
                override fun lookup(hostname: String): List<InetAddress> {
                    check(hostname == "api.anpfuel.example.invalid") { "Native trip test refused an unrelated host" }
                    return listOf(InetAddress.getByAddress(byteArrayOf(127, 0, 0, 1)))
                }
            }).followRedirects(false).followSslRedirects(false).retryOnConnectionFailure(false).build()
        } catch (_: Exception) { throw AssertionError("Disposable trip trust anchor is invalid") }
    }
}
