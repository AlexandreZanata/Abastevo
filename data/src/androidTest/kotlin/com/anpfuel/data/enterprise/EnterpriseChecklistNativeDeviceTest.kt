package com.anpfuel.data.enterprise

import android.content.Context
import android.os.Process
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.checklist.EnterpriseChecklistProducers
import com.anpfuel.application.enterprise.checklist.EnterpriseChecklistService
import com.anpfuel.application.enterprise.queue.EnterpriseQueueRegistry
import com.anpfuel.application.enterprise.queue.EnterpriseQueueService
import com.anpfuel.application.portable.*
import com.anpfuel.data.di.EnterpriseModule
import com.anpfuel.data.enterprise.checklist.EnterpriseChecklistHttpGateway
import com.anpfuel.data.enterprise.checklist.ProtectedChecklistStore
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.data.local.auth.AccountHttpApi
import com.anpfuel.data.local.auth.AndroidAnonymousDeviceKeys
import com.anpfuel.data.local.auth.AndroidKeystoreKeys
import com.anpfuel.data.local.auth.KeystoreAccountKey
import com.anpfuel.data.local.auth.KeystoreSessionStore
import com.anpfuel.data.remote.AnonymousProofHttpClient
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.PhotoProofTransport
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistAnswer
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistContext
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistLocalException
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistLocalFailure
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistOutcome
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistPresentation
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistRunOperations
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistRunView
import com.anpfuel.domain.enterprise.maintenance.checklist.PolicyMode
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import com.anpfuel.domain.enterprise.sync.SyncLimits
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
 * Opt-in seven-process EMP-CHECK-002 acceptance against the private ordinary PostGIS/HTTPS checklist fixture
 * (orgsync_checklist_native_fixture_test.go), with the product's real protected store, queue, producer, signer, company
 * cookie client and gateway. Stage 1 caches the DEPARTURE template online; the driver then force-stops this target and
 * removes the route; stages 2 and 3 fill and complete a run offline across a process death; the route returns for stage 4
 * (delivery and the company's agreeing recheck); stage 5 caches the WARN revision; offline again, the owner restores BLOCK
 * and stage 6 completes on the cached WARN; stage 7 shows the company's recalculation.
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseChecklistNativeDeviceTest {
    private val context: Context get() = InstrumentationRegistry.getInstrumentation().targetContext

    @Test
    fun stage1OnlineCachesTheDepartureTemplate(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = true)
        try {
            assertEquals("Unexpected previous native checklist stage", 0, graph.preferences.getInt(STAGE_NUMBER, 0))
            graph.establishCustody()
            val cached = graph.service.fetch(graph.source.account(), fixture.summary(), fixture.asset, ChecklistContext.DEPARTURE)
            assertTrue(cached.required)
            val template = cached.template!!
            assertEquals(fixture.template, template.templateId)
            assertEquals(listOf("Luzes", "Freios", "Pneus"), template.items.map { it.label })
            assertEquals(listOf(false, true, false), template.items.map { it.critical })
            assertEquals(listOf(true, false, false), template.items.map { it.optional })
            assertEquals(PolicyMode.BLOCK, cached.policy!!.mode)
            assertEquals(1L, cached.policy!!.revision)
            val stored = graph.book().applicable(fixture.asset, ChecklistContext.DEPARTURE)!!
            assertEquals(cached.evaluatedAt, stored.evaluatedAt)
            assertEquals(1, graph.applicableReads.get())
            assertEquals(0, graph.operationsSent.get())
            graph.mark(1)
        } finally { graph.close() }
    }

    @Test
    fun stage2OfflineStartsAndSavesTheFirstAnswerDurably(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(1)
            // The cached template is read with no network at all.
            assertTrue(graph.book().applicable(fixture.asset, ChecklistContext.DEPARTURE)!!.required)
            val run = graph.service.start(graph.source.account(), fixture.partition(), fixture.asset, ChecklistContext.DEPARTURE)
            graph.service.answer(graph.source.account(), fixture.partition(), run, 1, ChecklistAnswer.OK, null)
            assertEquals(ChecklistAnswer.OK, graph.book().run(run)!!.answers[0].answer)
            assertEquals("Nothing reaches the source offline", 0, graph.requests.get())
            assertTrue(graph.preferences.edit().putString(RUN, run).commit())
            graph.mark(2)
        } finally { graph.close() }
    }

    @Test
    fun stage3OfflineAfterProcessDeathCompletesBlocking(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(2)
            val run = requireNotNull(graph.preferences.getString(RUN, null))
            val restored = graph.book().openRun(fixture.asset, ChecklistContext.DEPARTURE)!!
            assertEquals("Process death lost the run", run, restored.runId)
            assertEquals("Process death lost the answer", ChecklistAnswer.OK, restored.answers[0].answer)
            val account = graph.source.account()
            graph.service.answer(account, fixture.partition(), run, 2, ChecklistAnswer.NOT_OK, "Freios rangendo")
            graph.service.answer(graph.source.account(), fixture.partition(), run, 3, ChecklistAnswer.OK, null)
            graph.service.complete(graph.source.account(), fixture.partition(), run, acknowledged = false)
            val view = graph.view(run)
            assertEquals(ChecklistOutcome.BLOCKING, view.outcome)
            assertEquals(listOf(2), view.blocking)
            assertEquals("Freios", view.run.template.items[view.blocking.single() - 1].label)
            assertEquals(EnterpriseSyncStatus.SAVED_ON_DEVICE, view.sync)
            assertTrue("The trip start of this vehicle is disabled on the device", ChecklistPresentation.startBlocked(graph.departure()))
            // Without a route a delivery attempt sends nothing and changes nothing.
            graph.delivery.dispatch(graph.source.account(), fixture.partition())
            assertEquals(0, graph.operationsSent.get())
            assertEquals(EnterpriseSyncStatus.SAVED_ON_DEVICE, graph.view(run).sync)
            graph.assertProtectedSurfaces(listOf(run, "Freios rangendo", "checklist-book", ChecklistRunOperations.COMPLETE, "Checklist de saída"))
            graph.mark(3)
        } finally { graph.close() }
    }

    @Test
    fun stage4ReconnectionDeliversAndReadsTheAgreeingRecheck(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(3)
            val run = requireNotNull(graph.preferences.getString(RUN, null))
            val accepted = graph.deliverUntilAccepted(run)
            assertEquals(EnterpriseSyncStatus.ACCEPTED, accepted.sync)
            assertTrue(graph.service.readServer(graph.source.account(), fixture.summary(), fixture.asset) >= 1)
            val view = graph.view(run)
            val server = view.run.server!!
            assertEquals(ChecklistOutcome.BLOCKING, server.outcome)
            assertEquals(ChecklistOutcome.BLOCKING, server.recheckOutcome)
            assertFalse(view.differs)
            assertEquals(1L, server.serverPolicyRevision)
            val sent = graph.operationsSent.get()
            graph.delivery.dispatch(graph.source.deliveryAccount(), fixture.partition())
            assertEquals("An accepted run was sent again", sent, graph.operationsSent.get())
            graph.mark(4)
        } finally { graph.close() }
    }

    @Test
    fun stage5OnlineCachesTheWarnRevision(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(4)
            assertTrue("The owner's WARN change must precede this read", File(context.noBackupFilesDir, "enterprise-native-checklist-warned").exists())
            val cached = graph.service.fetch(graph.source.account(), fixture.summary(), fixture.asset, ChecklistContext.DEPARTURE)
            assertEquals(PolicyMode.WARN, cached.policy!!.mode)
            assertEquals(2L, cached.policy!!.revision)
            graph.mark(5)
        } finally { graph.close() }
    }

    @Test
    fun stage6OfflineCompletesWithIssuesOnTheCachedWarn(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(5)
            val partition = fixture.partition()
            val run = graph.service.start(graph.source.account(), partition, fixture.asset, ChecklistContext.DEPARTURE)
            for ((position, answer) in listOf(1 to ChecklistAnswer.OK, 2 to ChecklistAnswer.NOT_OK, 3 to ChecklistAnswer.OK)) {
                graph.service.answer(graph.source.account(), partition, run, position, answer, null)
            }
            val refused = try { graph.service.complete(graph.source.account(), partition, run, acknowledged = false); null }
                catch (refusal: ChecklistLocalException) { refusal.failure }
            assertEquals("WARN with a critical issue asks for the acknowledgement", ChecklistLocalFailure.ACKNOWLEDGEMENT_REQUIRED, refused)
            graph.service.complete(graph.source.account(), partition, run, acknowledged = true)
            val view = graph.view(run)
            assertEquals(ChecklistOutcome.COMPLETED_WITH_ISSUES, view.outcome)
            assertTrue(view.run.acknowledged)
            assertEquals(2L, view.run.policy.revision)
            assertEquals(EnterpriseSyncStatus.SAVED_ON_DEVICE, view.sync)
            assertEquals("Nothing reaches the source offline", 0, graph.requests.get())
            assertTrue(graph.preferences.edit().putString(SECOND_RUN, run).commit())
            graph.mark(6)
        } finally { graph.close() }
    }

    @Test
    fun stage7ReconnectionShowsTheCompanysRecalculation(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(6)
            val run = requireNotNull(graph.preferences.getString(SECOND_RUN, null))
            assertEquals(EnterpriseSyncStatus.ACCEPTED, graph.deliverUntilAccepted(run).sync)
            assertTrue(graph.service.readServer(graph.source.account(), fixture.summary(), fixture.asset) >= 1)
            val view = graph.view(run)
            val server = view.run.server!!
            assertTrue("A empresa recalculou o resultado", view.differs)
            assertEquals(ChecklistOutcome.COMPLETED_WITH_ISSUES, view.run.deviceOutcome)
            assertEquals(ChecklistOutcome.BLOCKING, server.recheckOutcome)
            assertEquals(ChecklistOutcome.BLOCKING, view.outcome)
            assertEquals(3L, server.serverPolicyRevision)
            assertTrue(server.reviewRequired)
            assertTrue("The company's BLOCKING result disables the trip start on the device", ChecklistPresentation.startBlocked(graph.departure()))
            // The first run is unchanged and nothing was refused.
            assertEquals(ChecklistOutcome.BLOCKING, graph.view(requireNotNull(graph.preferences.getString(RUN, null))).outcome)
            assertTrue(graph.entries().values.all { it.acknowledgement?.result == com.anpfuel.domain.enterprise.sync.SyncResult.ACCEPTED })
        } finally {
            graph.cleanup()
            graph.close()
        }
    }

    private fun fixtureFile() = File(context.noBackupFilesDir, "enterprise-native-checklist-fixture.json")

    private class SourceFixture(val origin: String, val certificate: String, val session: PortableAuth.Session, val organization: String,
        val member: String, val asset: String, val template: String) {
        fun partition() = EnterprisePartition.of(session.accountId, origin, organization)
        fun summary() = EnterpriseContextSummary(partition(), member, "Synthetic checklists")
        override fun toString(): String = "ChecklistSourceFixture(redacted)"
    }

    private class Graph(private val context: Context, private val fixture: SourceFixture, seed: Boolean) : Closeable {
        val preferences = context.getSharedPreferences("enterprise-native-checklist", Context.MODE_PRIVATE)
        private val fence = EnterpriseSessionFence()
        private val platformKeys = AndroidKeystoreKeys()
        private val sessions = EnterpriseFencedSessionStore(KeystoreSessionStore(preferences, platformKeys, SESSION_ALIAS), fence)
        private val accountKeys = KeystoreAccountKey(preferences, platformKeys, BACKUP_ALIAS)
        init {
            if (seed) {
                assertNull("Unexpected existing native checklist session", sessions.load())
                sessions.save(fixture.session)
            }
            val current = sessions.load() ?: throw AssertionError("Retained native checklist session is unavailable")
            // The retained session is the fixture's own family: a refresh never meets another family (FLEET-013 incident).
            assertTrue("Retained source changed ownership", current.accountId == fixture.session.accountId && current.familyId == fixture.session.familyId)
        }
        /** Requests actually answered by the source (a refused connection is never counted). */
        val requests = AtomicInteger()
        val operationsSent = AtomicInteger()
        val applicableReads = AtomicInteger()
        val statuses: MutableList<Int> = java.util.Collections.synchronizedList(mutableListOf())
        private val http = fixtureHttp(fixture.certificate).newBuilder().addInterceptor { chain ->
            chain.proceed(chain.request()).also {
                requests.incrementAndGet()
                val path = chain.request().url.encodedPath
                if (path.endsWith("/sync/operations")) { operationsSent.incrementAndGet(); statuses += it.code }
                if (path.endsWith("/applicable")) applicableReads.incrementAndGet()
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
        private val queue = ProtectedEnterpriseQueue(persistence)
        val delivery = EnterpriseQueueService(source, queue, EnterpriseOperationHttpClient(http, source, signer, SyncLimits.DEFAULT),
            EnterpriseQueueRegistry(*EnterpriseChecklistProducers.all()), clock::read, entropy = { 0L })
        private val store = ProtectedChecklistStore(persistence)
        private val company = CompanyAccessClient(http, fixture.origin, sessions, auth, fence)
        val service = EnterpriseChecklistService(source, delivery::save, store, EnterpriseChecklistHttpGateway(company), { ZoneId.systemDefault() })

        suspend fun snapshot() = service.snapshot(source.account(), fixture.partition())
        suspend fun book() = requireNotNull(snapshot().book) { "No checklist book on this device" }
        suspend fun entries() = snapshot().entries
        suspend fun view(run: String): ChecklistRunView {
            val snapshot = snapshot()
            return ChecklistPresentation.view(snapshot.book!!.run(run)!!, snapshot.entries, service.now())
        }
        suspend fun departure(): ChecklistRunView? {
            val snapshot = snapshot()
            return ChecklistPresentation.latestDeparture(snapshot.book!!, snapshot.entries, fixture.asset, service.now())
        }
        suspend fun deliverUntilAccepted(run: String): ChecklistRunView {
            val diagnostics = StringBuilder()
            for (attempt in 1..20) {
                val result = try { delivery.dispatch(source.deliveryAccount(), fixture.partition()) }
                    catch (failure: Exception) { diagnostics.append("dispatch ").append(failure.javaClass.simpleName).append("; "); null }
                result?.let { diagnostics.append("attempted=${it.attempted} completed=${it.completed} retryable=${it.retryable} attention=${it.attention} ") }
                val view = view(run)
                diagnostics.append("sent=${operationsSent.get()} http=$statuses ${view.sync}/${view.code}; ")
                if (view.sync == EnterpriseSyncStatus.ACCEPTED) return view
                Thread.sleep(3_000)
            }
            throw AssertionError("The run was not accepted after reconnection: $diagnostics")
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
            val handoff = persistence.confirm(EnterpriseDeviceTarget(fixture.partition(), fixture.member), before, owner)
            local.select(local.consumeContact(owner, handoff, binding()), fixture.partition())
            assertTrue("Actual identity-v1 signing key was not created", deviceKeys.hasKey())
        }

        /** No checklist fact, label, note or credential appears outside encryption. */
        fun assertProtectedSurfaces(secrets: List<String>) {
            val token = EnterpriseVaultCodec.token(fixture.partition())
            for (generation in files.generations(token)) for (descriptor in listOf(false, true)) {
                val text = String(files.readGeneration(token, generation, descriptor) ?: continue, Charsets.ISO_8859_1)
                for (secret in secrets + fixture.session.accountId + fixture.session.accessToken)
                    assertFalse("A checklist value appeared outside encryption", text.contains(secret))
            }
        }

        fun cleanup() {
            assertTrue("Native checklist cleanup was incomplete", erasure.partitionRevoked(fixture.partition()))
            val identityAlias = AndroidAnonymousDeviceKeys.aliasFor(AuthFlow.AuthState.Active(fixture.session), fixture.origin)
            KeyStore.getInstance("AndroidKeyStore").apply {
                load(null); deleteEntry(identityAlias); deleteEntry(SESSION_ALIAS); deleteEntry(BACKUP_ALIAS)
            }
            assertTrue(preferences.edit().clear().commit())
            for (name in listOf("enterprise-native-checklist-fixture.json", "enterprise-native-checklist-warned")) {
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
        private const val STAGE = "checklist_stage_process"
        private const val STAGE_NUMBER = "checklist_stage_number"
        private const val RUN = "checklist_run_first"
        private const val SECOND_RUN = "checklist_run_second"
        private const val SESSION_ALIAS = "enterprise_native_checklist_session"
        private const val BACKUP_ALIAS = "enterprise_native_checklist_backup"

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
            SourceFixture(origin, certificate, session, EnterprisePartition.requireId(value.getString("org_id")),
                EnterprisePartition.requireId(value.getString("membership_id")), EnterprisePartition.requireId(value.getString("asset_id")),
                EnterprisePartition.requireId(value.getString("template_id")))
        } catch (_: Exception) { throw AssertionError("Disposable checklist fixture is invalid") }

        private fun fixtureHttp(pem: String): OkHttpClient = try {
            val certificate = CertificateFactory.getInstance("X.509").generateCertificate(pem.byteInputStream())
            val anchors = KeyStore.getInstance(KeyStore.getDefaultType()).apply { load(null); setCertificateEntry("disposable-checklist", certificate) }
            val trust = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply { init(anchors) }
                .trustManagers.single() as X509TrustManager
            val tls = SSLContext.getInstance("TLS").apply { init(null, arrayOf(trust), null) }
            OkHttpClient.Builder().sslSocketFactory(tls.socketFactory, trust).dns(object : Dns {
                override fun lookup(hostname: String): List<InetAddress> {
                    check(hostname == "api.anpfuel.example.invalid") { "Native checklist test refused an unrelated host" }
                    return listOf(InetAddress.getByAddress(byteArrayOf(127, 0, 0, 1)))
                }
            }).followRedirects(false).followSslRedirects(false).retryOnConnectionFailure(false).build()
        } catch (_: Exception) { throw AssertionError("Disposable checklist trust anchor is invalid") }
    }
}
