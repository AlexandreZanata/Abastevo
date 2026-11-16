package com.anpfuel.data.enterprise

import android.content.Context
import android.os.Process
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.fleet.EnterpriseReferenceService
import com.anpfuel.application.enterprise.queue.EnterpriseQueueRegistry
import com.anpfuel.application.enterprise.queue.EnterpriseQueueService
import com.anpfuel.application.enterprise.trip.EnterpriseMissionProducers
import com.anpfuel.application.enterprise.trip.EnterpriseMissionService
import com.anpfuel.application.enterprise.trip.EnterpriseTripProducers
import com.anpfuel.application.enterprise.trip.EnterpriseTripService
import com.anpfuel.application.enterprise.trip.EnterpriseTripStartHints
import com.anpfuel.application.enterprise.trip.MissionUnreachable
import com.anpfuel.application.enterprise.trip.TripCapture
import com.anpfuel.application.enterprise.trip.TripMeterChoice
import com.anpfuel.application.portable.*
import com.anpfuel.data.di.EnterpriseModule
import com.anpfuel.data.enterprise.fleet.EnterpriseReferenceHttpClient
import com.anpfuel.data.enterprise.fleet.ProtectedReferenceCacheStore
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.data.enterprise.trip.EnterpriseMissionHttpGateway
import com.anpfuel.data.enterprise.trip.ProtectedMissionStore
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
import com.anpfuel.domain.enterprise.trip.MissionAction
import com.anpfuel.domain.enterprise.trip.MissionAnswerState
import com.anpfuel.domain.enterprise.trip.MissionInboxView
import com.anpfuel.domain.enterprise.trip.MissionLocalFailure
import com.anpfuel.domain.enterprise.trip.TripAssetLabel
import com.anpfuel.domain.enterprise.trip.TripCommand
import com.anpfuel.domain.enterprise.trip.TripLocalState
import com.anpfuel.domain.enterprise.trip.TripMissionOperations
import com.anpfuel.domain.enterprise.trip.TripMissionResponse
import com.anpfuel.domain.enterprise.trip.TripMissionState
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
 * Opt-in five-process EMP-TRIP-021 acceptance against the private ordinary PostGIS/HTTPS mission fixture
 * (orgsync_mission_native_fixture_test.go), with the product's real protected store, queue, producers, signer, company cookie
 * client and own-list gateway. Stage 1 is online (custody of both organizations, the reference pull and the missions copy);
 * the driver removes the route; stage 2 answers both missions offline; the owner reassigns the second mission; stage 3 keeps
 * both answers across a process death; the route returns for stage 4 (one answer confirmed, the reassigned one denied alone,
 * the planned trip started, paused and resumed) and stage 5 (the finish completes the mission).
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseMissionNativeDeviceTest {
    private val context: Context get() = InstrumentationRegistry.getInstrumentation().targetContext

    @Test
    fun stage1OnlineCustodyReferencesAndMissionsCopy(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = true)
        try {
            assertEquals("Unexpected previous native mission stage", 0, graph.preferences.getInt(STAGE_NUMBER, 0))
            graph.establishCustody()
            val (a, b) = fixture.organizations
            assertNotNull(graph.references.refresh(graph.source.deliveryAccount(), a.partition(fixture)).cache)
            val inbox = graph.missions.refresh(graph.source.account(), a.context(fixture))
            assertTrue(inbox.refreshed); assertFalse(inbox.stale); assertEquals(0L, inbox.ageMillis!! / 60_000)
            assertEquals(fixture.missions.toSet(), inbox.missions.map { it.missionId }.toSet())
            for (mission in inbox.missions) {
                assertEquals(TripMissionState.ASSIGNED, mission.state)
                assertEquals(3L, mission.item!!.revision)
                assertEquals(a.asset, mission.item!!.assetId)
                assertEquals(setOf(MissionAction.ACCEPT, MissionAction.DECLINE), mission.actions)
            }
            assertEquals("Cliente Norte, doca 3", inbox.missions.single { it.missionId == fixture.missions[0] }.item!!.plan!!.destinationText)
            // Organization B's own list is its own: empty, with its own age.
            val other = graph.missions.refresh(graph.source.account(), b.context(fixture))
            assertTrue(other.refreshed && other.missions.isEmpty())
            assertEquals(2, graph.ownLists.get())
            graph.mark(1)
        } finally { graph.close() }
    }

    @Test
    fun stage2OfflineAnswersAreDurableAndOnlineOnlyRefreshSaysSo(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(1)
            val (a, b) = fixture.organizations
            for (mission in fixture.missions) graph.missions.respond(graph.source.account(), a.context(fixture), mission, TripMissionResponse.ACCEPT, null)
            val inbox = graph.inbox(a)
            for (mission in inbox.missions) {
                assertEquals("An offline answer is only saved", MissionAnswerState.SAVED, mission.answer)
                assertEquals(TripMissionState.ASSIGNED, mission.state)
                assertEquals("No planned start before the company confirms", MissionLocalFailure.NOT_CONFIRMED, mission.startBlocked)
                assertTrue(mission.actions.isEmpty())
            }
            // Online only: the refresh reaches nothing, keeps the copy and its age (BR-OFFL-016/017).
            val failure = try { graph.missions.refresh(graph.source.account(), a.context(fixture)); null } catch (unreachable: MissionUnreachable) { unreachable.error }
            assertNotNull("An offline refresh is never reported done", failure)
            assertEquals(2, graph.inbox(a).missions.size)
            // A delivery attempt without a route sends nothing; organization B shows and queues nothing of A.
            graph.delivery.dispatch(graph.source.account(), a.partition(fixture))
            assertEquals(0, graph.operationsSent.get())
            assertTrue(graph.inbox(b).missions.isEmpty())
            assertTrue(graph.answers(b).isEmpty())
            graph.mark(2)
        } finally { graph.close() }
    }

    @Test
    fun stage3OfflineAfterProcessDeathAndReassignmentKeepsBothAnswers(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(2)
            assertTrue("The owner's reassignment must precede this stage", File(context.noBackupFilesDir, "enterprise-native-mission-reassigned").exists())
            val (a, _) = fixture.organizations
            val inbox = graph.inbox(a)
            assertEquals(listOf(MissionAnswerState.SAVED, MissionAnswerState.SAVED), inbox.missions.map { it.answer })
            val answers = graph.answers(a)
            assertEquals(2, answers.size)
            for (entry in answers.values) {
                // Sealed to the creation partition: organization, membership and enrollment of A (BR-OFFL-003).
                assertEquals(a.id, entry.operation.binding.organizationId)
                assertEquals(a.member, entry.operation.membershipId)
                assertEquals(3L, TripMissionOperations.read(entry.operation).revision)
                assertNull(entry.acknowledgement)
            }
            graph.assertProtectedSurfaces(fixture.missions + listOf("Cliente Norte", "Cliente Sul", "mission-inbox", TripMissionOperations.RESPOND))
            graph.mark(3)
        } finally { graph.close() }
    }

    @Test
    fun stage4ReconnectionConfirmsOneDeniesTheOtherAloneAndStartsThePlannedTrip(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(3)
            val (a, _) = fixture.organizations
            val (first, reassigned) = fixture.missions
            graph.deliverUntil(a) { inbox -> inbox.missions.first { it.missionId == first }.answer == MissionAnswerState.ACCEPTED }
            val answers = graph.answers(a)
            val denied = answers.values.single { TripMissionOperations.read(it.operation).missionId == reassigned }
            assertNull("The denied answer is never shown accepted", denied.acknowledgement)
            assertTrue("The denied answer stays saved and retried", denied.retry.attempts >= 1)
            assertTrue("A denial is answered only to a batch of one answer", graph.deniedBatchesAlone())
            // The answer's own receipt proves the acceptance; a refresh shows the company's list.
            val refreshed = graph.missions.refresh(graph.source.account(), a.context(fixture))
            assertEquals(TripMissionState.ACCEPTED, refreshed.missions.single { it.missionId == first }.state)
            assertEquals(MissionAnswerState.NOT_CONFIRMED_REMOVED, refreshed.missions.single { it.missionId == reassigned }.answer)
            assertEquals(setOf(MissionAction.START_TRIP), refreshed.missions.single { it.missionId == first }.actions)
            // The planned trip on the mission's vehicle, with the mission reference, then pause and resume.
            val cached = requireNotNull(graph.references.cached(graph.source.account(), a.partition(fixture)))
            val snapshot = MyAssets.present(cached, graph.clock.read())
            val choice = requireNotNull(MyAssets.choose(a.id, snapshot, a.asset, warningAccepted = true))
            val asset = snapshot.assets.single { it.asset.id == a.asset }.asset
            val meter = choice.meters.single()
            val departure = TripCapture.Reading(TripMeterChoice(meter.id, meter.segmentId, MeterKind.ODOMETER, meter.modulus), MeterEntryForm.KM, "184320")
            graph.trips.start(graph.source.account(), a.context(fixture), choice, TripAssetLabel(asset.name, asset.fleetNumber, asset.plate), departure,
                missionId = first)
            val trip = graph.trip(a)
            assertEquals(first, trip.missionId)
            graph.trips.lifecycle(graph.source.account(), a.partition(fixture), trip.trip.tripId, TripCommand.PAUSE, "Almoço")
            graph.trips.lifecycle(graph.source.account(), a.partition(fixture), trip.trip.tripId, TripCommand.RESUME, null)
            assertEquals(MissionLocalFailure.ALREADY_STARTED, graph.inbox(a).missions.single { it.missionId == first }.startBlocked)
            graph.deliverUntil(a) { graph.trip(a).let { view -> (listOf(view.start) + view.lifecycle).all { it.sync == EnterpriseSyncStatus.ACCEPTED } } }
            assertEquals(TripMissionState.IN_PROGRESS, graph.missions.refresh(graph.source.account(), a.context(fixture))
                .missions.single { it.missionId == first }.state)
            assertTrue(graph.preferences.edit().putString(TRIP, trip.trip.tripId).commit())
            graph.mark(4)
        } finally { graph.close() }
    }

    @Test
    fun stage5TheFinishCompletesTheMission(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(4)
            val (a, _) = fixture.organizations
            val tripId = requireNotNull(graph.preferences.getString(TRIP, null))
            val view = graph.trip(a)
            assertEquals(tripId, view.trip.tripId)
            assertEquals(TripLocalState.IN_PROGRESS, view.trip.state)
            val meter = graph.departureMeter(view)
            graph.trips.finish(graph.source.account(), a.partition(fixture), tripId, TripCapture.Reading(meter, MeterEntryForm.KM, "184610"))
            graph.deliverUntil(a) { graph.trip(a).finish?.sync == EnterpriseSyncStatus.ACCEPTED }
            val inbox = graph.missions.refresh(graph.source.account(), a.context(fixture))
            assertEquals(TripMissionState.COMPLETED, inbox.missions.single { it.missionId == fixture.missions[0] }.state)
            assertEquals(setOf(MissionAction.OPEN_TRIP), inbox.missions.single { it.missionId == fixture.missions[0] }.actions)
        } finally {
            graph.cleanup()
            graph.close()
        }
    }

    private fun fixtureFile() = File(context.noBackupFilesDir, "enterprise-native-mission-fixture.json")

    private class Organization(val id: String, val member: String, val employee: String, val asset: String) {
        fun partition(fixture: SourceFixture) = EnterprisePartition.of(fixture.session.accountId, fixture.origin, id)
        fun context(fixture: SourceFixture) = EnterpriseContextSummary(partition(fixture), member, "Synthetic missions", employee)
        override fun toString(): String = "MissionFixtureOrganization(redacted)"
    }
    private class SourceFixture(val origin: String, val certificate: String, val session: PortableAuth.Session, val organizations: List<Organization>,
        val missions: List<String>) {
        override fun toString(): String = "MissionSourceFixture(redacted)"
    }

    private class Graph(private val context: Context, private val fixture: SourceFixture, seed: Boolean) : Closeable {
        val preferences = context.getSharedPreferences("enterprise-native-mission", Context.MODE_PRIVATE)
        private val fence = EnterpriseSessionFence()
        private val platformKeys = AndroidKeystoreKeys()
        private val sessions = EnterpriseFencedSessionStore(KeystoreSessionStore(preferences, platformKeys, SESSION_ALIAS), fence)
        private val accountKeys = KeystoreAccountKey(preferences, platformKeys, BACKUP_ALIAS)
        init {
            if (seed) {
                assertNull("Unexpected existing native mission session", sessions.load())
                sessions.save(fixture.session)
            }
            val current = sessions.load() ?: throw AssertionError("Retained native mission session is unavailable")
            assertTrue("Retained source changed ownership", current.accountId == fixture.session.accountId && current.familyId == fixture.session.familyId)
        }
        /** Intake answers actually received from the source (a refused connection is never counted), with each batch's size. */
        val operationsSent = AtomicInteger()
        val ownLists = AtomicInteger()
        private val batches: MutableList<Pair<Int, Int>> = java.util.Collections.synchronizedList(mutableListOf())
        private val http = fixtureHttp(fixture.certificate).newBuilder().addInterceptor { chain ->
            val request = chain.request()
            val size = if (request.url.encodedPath.endsWith("/sync/operations")) request.body?.let { body ->
                val buffer = okio.Buffer().also { body.writeTo(it) }
                JSONObject(buffer.readUtf8()).getJSONArray("operations").length()
            } ?: 0 else 0
            chain.proceed(request).also {
                val path = request.url.encodedPath
                if (path.endsWith("/sync/operations")) { operationsSent.incrementAndGet(); batches += it.code to size }
                if (path.endsWith("/missions/mine/list")) ownLists.incrementAndGet()
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
            EnterpriseQueueRegistry(*EnterpriseTripProducers.all(hints), *EnterpriseMissionProducers.all()), clock::read, entropy = { 0L })
        private val tripStore = ProtectedTripStore(persistence)
        private val missionStore = ProtectedMissionStore(persistence)
        private val company = CompanyAccessClient(http, fixture.origin, sessions, auth, fence)
        val trips = EnterpriseTripService(source, delivery::save, tripStore, hints, clock::read, { ZoneId.systemDefault() }, missions = missionStore)
        val missions = EnterpriseMissionService(source, delivery::save, missionStore, EnterpriseMissionHttpGateway(company), tripStore, clock::read,
            { ZoneId.systemDefault() })

        suspend fun inbox(organization: Organization): MissionInboxView = missions.inbox(source.account(), organization.partition(fixture))
        suspend fun answers(organization: Organization) = missionStore.read(source.attempt(source.account(), organization.partition(fixture))).entries
        suspend fun trip(organization: Organization): TripView = trips.trips(source.account(), organization.partition(fixture)).single()
        fun departureMeter(view: TripView): TripMeterChoice {
            val evidence = (view.start.fact as com.anpfuel.domain.enterprise.trip.TripOperationFact.Start).evidence
                as com.anpfuel.domain.enterprise.trip.TripEvidence.Reading
            return TripMeterChoice(evidence.meterId, evidence.segmentId, evidence.value.kind, null)
        }
        /** Every refused batch held exactly one operation: the denied answer. */
        fun deniedBatchesAlone(): Boolean = synchronized(batches) { batches.any { it.first != 200 } && batches.filter { it.first != 200 }.all { it.second == 1 } }
        suspend fun deliverUntil(organization: Organization, done: suspend (MissionInboxView) -> Boolean) {
            val diagnostics = StringBuilder()
            for (attempt in 1..30) {
                val result = try { delivery.dispatch(source.deliveryAccount(), organization.partition(fixture)) }
                    catch (failure: Exception) { diagnostics.append("dispatch ").append(failure.javaClass.simpleName).append("; "); null }
                result?.let { diagnostics.append("attempted=${it.attempted} completed=${it.completed} retryable=${it.retryable} ") }
                diagnostics.append("batches=${synchronized(batches) { batches.toList() }}; ")
                if (done(inbox(organization))) return
                Thread.sleep(2_000)
            }
            throw AssertionError("The expected receipts did not arrive: $diagnostics")
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

        /** No mission fact, text or credential appears outside encryption. */
        fun assertProtectedSurfaces(secrets: List<String>) {
            for (organization in fixture.organizations) {
                val token = EnterpriseVaultCodec.token(organization.partition(fixture))
                for (generation in files.generations(token)) for (descriptor in listOf(false, true)) {
                    val text = String(files.readGeneration(token, generation, descriptor) ?: continue, Charsets.ISO_8859_1)
                    for (secret in secrets + fixture.session.accountId + fixture.session.accessToken)
                        assertFalse("A mission value appeared outside encryption", text.contains(secret))
                }
            }
        }

        fun cleanup() {
            fixture.organizations.forEach { assertTrue("Native mission cleanup was incomplete", erasure.partitionRevoked(it.partition(fixture))) }
            val identityAlias = AndroidAnonymousDeviceKeys.aliasFor(AuthFlow.AuthState.Active(fixture.session), fixture.origin)
            KeyStore.getInstance("AndroidKeyStore").apply {
                load(null); deleteEntry(identityAlias); deleteEntry(SESSION_ALIAS); deleteEntry(BACKUP_ALIAS)
            }
            assertTrue(preferences.edit().clear().commit())
            for (name in listOf("enterprise-native-mission-fixture.json", "enterprise-native-mission-reassigned")) {
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
        private const val STAGE = "mission_stage_process"
        private const val STAGE_NUMBER = "mission_stage_number"
        private const val TRIP = "mission_trip"
        private const val SESSION_ALIAS = "enterprise_native_mission_session"
        private const val BACKUP_ALIAS = "enterprise_native_mission_backup"

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
                    Organization(EnterprisePartition.requireId(it.getString("org_id")), EnterprisePartition.requireId(it.getString("membership_id")),
                        EnterprisePartition.requireId(it.getString("employee_id")), EnterprisePartition.requireId(it.getString("asset_id")))
                } }
            }
            val missions = value.getJSONArray("missions").let { rows -> (0 until rows.length()).map { EnterprisePartition.requireId(rows.getString(it)) } }
            require(organizations.map { it.id }.distinct().size == 2 && missions.size == 2)
            SourceFixture(origin, certificate, session, organizations, missions)
        } catch (_: Exception) { throw AssertionError("Disposable mission fixture is invalid") }

        private fun fixtureHttp(pem: String): OkHttpClient = try {
            val certificate = CertificateFactory.getInstance("X.509").generateCertificate(pem.byteInputStream())
            val anchors = KeyStore.getInstance(KeyStore.getDefaultType()).apply { load(null); setCertificateEntry("disposable-mission", certificate) }
            val trust = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply { init(anchors) }
                .trustManagers.single() as X509TrustManager
            val tls = SSLContext.getInstance("TLS").apply { init(null, arrayOf(trust), null) }
            OkHttpClient.Builder().sslSocketFactory(tls.socketFactory, trust).dns(object : Dns {
                override fun lookup(hostname: String): List<InetAddress> {
                    check(hostname == "api.anpfuel.example.invalid") { "Native mission test refused an unrelated host" }
                    return listOf(InetAddress.getByAddress(byteArrayOf(127, 0, 0, 1)))
                }
            }).followRedirects(false).followSslRedirects(false).retryOnConnectionFailure(false).build()
        } catch (_: Exception) { throw AssertionError("Disposable mission trust anchor is invalid") }
    }
}
