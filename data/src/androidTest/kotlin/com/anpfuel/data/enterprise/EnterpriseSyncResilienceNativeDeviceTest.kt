package com.anpfuel.data.enterprise

import android.content.Context
import android.os.Process
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.attachment.EnterpriseAttachmentService
import com.anpfuel.application.enterprise.attachment.EnterpriseReceipts
import com.anpfuel.application.enterprise.attachment.ReceiptDraft
import com.anpfuel.application.enterprise.expense.EnterpriseExpenseProducer
import com.anpfuel.application.enterprise.expense.EnterpriseExpenseService
import com.anpfuel.application.enterprise.expense.ExpenseFlow
import com.anpfuel.application.enterprise.expense.ExpenseForm
import com.anpfuel.application.enterprise.expense.ExpensePayer
import com.anpfuel.application.enterprise.fleet.EnterpriseReferenceService
import com.anpfuel.application.enterprise.fueling.EnterpriseFuelingProducer
import com.anpfuel.application.enterprise.fueling.EnterpriseFuelingService
import com.anpfuel.application.enterprise.fueling.FuelingFlow
import com.anpfuel.application.enterprise.fueling.FuelingForm
import com.anpfuel.application.enterprise.queue.EnterpriseQueueRegistry
import com.anpfuel.application.enterprise.queue.EnterpriseQueueService
import com.anpfuel.application.enterprise.trip.EnterpriseTripProducers
import com.anpfuel.application.enterprise.trip.EnterpriseTripService
import com.anpfuel.application.enterprise.trip.EnterpriseTripStartHints
import com.anpfuel.application.enterprise.trip.TripCapture
import com.anpfuel.application.enterprise.trip.TripMeterChoice
import com.anpfuel.application.portable.*
import com.anpfuel.data.di.EnterpriseModule
import com.anpfuel.data.enterprise.attachment.AndroidReceiptOriginals
import com.anpfuel.data.enterprise.attachment.EnterpriseAttachmentHttpClient
import com.anpfuel.data.enterprise.attachment.ProtectedAttachmentStore
import com.anpfuel.data.enterprise.expense.ProtectedExpenseStore
import com.anpfuel.data.enterprise.fleet.EnterpriseReferenceHttpClient
import com.anpfuel.data.enterprise.fleet.ProtectedReferenceCacheStore
import com.anpfuel.data.enterprise.fueling.ProtectedFuelingStore
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
import com.anpfuel.domain.enterprise.attachment.AttachmentLabel
import com.anpfuel.domain.enterprise.attachment.AttachmentPolicy
import com.anpfuel.domain.enterprise.attachment.ReceiptInspection
import com.anpfuel.domain.enterprise.attachment.ReceiptIntake
import com.anpfuel.domain.enterprise.attachment.ReceiptLegibility
import com.anpfuel.domain.enterprise.attachment.ReceiptSource
import com.anpfuel.domain.enterprise.fleet.MyAssets
import com.anpfuel.domain.enterprise.fueling.FillLevel
import com.anpfuel.domain.enterprise.fueling.FuelProduct
import com.anpfuel.domain.enterprise.fueling.Payer
import com.anpfuel.domain.enterprise.fueling.PaymentMethod
import com.anpfuel.domain.enterprise.meter.MeterEntryForm
import com.anpfuel.domain.enterprise.meter.MeterKind
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import com.anpfuel.domain.enterprise.sync.QueueRetryPolicy
import com.anpfuel.domain.enterprise.sync.SyncLimits
import com.anpfuel.domain.enterprise.trip.TripAssetLabel
import com.anpfuel.domain.enterprise.trip.TripLogbookType
import com.anpfuel.domain.enterprise.trip.TripView
import com.anpfuel.domain.portable.PortableAuth
import java.io.Closeable
import java.io.File
import java.net.InetAddress
import java.security.KeyStore
import java.security.cert.CertificateFactory
import java.time.Instant
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
 * Opt-in four-process EMP-OFFL-013 acceptance (B-BR-EMP-540..543, BUC-EMP-278) against the private ordinary
 * PostGIS/HTTPS/MinIO receipt fixture (orgsync_receipt_native_fixture_test.go, with its one-shot lost-answer intake
 * fault), with the product's real protected store, queue, trip, fueling and expense owners, the EMP-DOC-004 capture
 * path, the attachment service, signer and HTTP clients. Stage 1 is online (custody and the reference pull); stage 2
 * records a field day offline (trip start, a logbook stop, a fueling and a toll with their receipts, the finish);
 * stage 3 proves it survived the process death; stage 4 restores the route with the intake answer lost after commit
 * and the first receipt transfer truncated: nothing is acknowledged from the lost answer, the resend is accepted once,
 * the truncated receipt stays sending and is stored on retry, and nothing is sent twice.
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseSyncResilienceNativeDeviceTest {
    private val context: Context get() = InstrumentationRegistry.getInstrumentation().targetContext

    @Test
    fun stage1OnlineCustodyAndReferencePull(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = true)
        try {
            assertEquals("Unexpected previous native resilience stage", 0, graph.preferences.getInt(STAGE_NUMBER, 0))
            graph.establishCustody()
            val (a, _) = fixture.organizations
            val cache = requireNotNull(graph.references.refresh(graph.source.deliveryAccount(), a.partition(fixture)).cache)
            assertEquals(listOf(a.meter!!), cache.assets.single { it.id == a.asset }.meters.map { it.id })
            graph.mark(1)
        } finally { graph.close() }
    }

    @Test
    fun stage2OfflineFieldDayIsSavedBeforeAnyDelivery(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(1)
            val (a, _) = fixture.organizations
            val cached = requireNotNull(graph.references.cached(graph.source.account(), a.partition(fixture)))
            val snapshot = MyAssets.present(cached, graph.clock.read())
            val choice = requireNotNull(MyAssets.choose(a.id, snapshot, a.asset, warningAccepted = false))
            val asset = snapshot.assets.single { it.asset.id == a.asset }.asset
            val meter = choice.meters.single()
            fun reading(km: String) = TripCapture.Reading(TripMeterChoice(meter.id, meter.segmentId, MeterKind.ODOMETER, meter.modulus), MeterEntryForm.KM, km)
            graph.service.start(graph.source.account(), a.context(fixture), choice, TripAssetLabel(asset.name, asset.fleetNumber, asset.plate), reading("184320"))
            val trip = graph.trips(a).single().trip.tripId
            graph.service.log(graph.source.account(), a.partition(fixture), trip, TripLogbookType.STOP, null)
            val fuelings = graph.fuelingFlow()
            fuelings.open(a.context(fixture), graph.fence.epoch)
            fuelings.record(choice, FuelingForm(FuelProduct.DIESEL_S10, "50", "324,50", null, "6,490", FillLevel.PARTIAL, Payer.COMPANY, PaymentMethod.FUEL_CARD,
                reading("184532"), "Posto sintético"), graph.draft(receiptJpeg(seed = 1), ReceiptSource.CAMERA), graph.fence.epoch, fuelings.state.value.generation)
            assertNotNull("The fueling was not saved", fuelings.state.value.lastSaved)
            val expenses = graph.expenseFlow()
            expenses.open(a.context(fixture), graph.fence.epoch)
            expenses.record(choice, ExpenseForm("TOLL", "12,50", "12,50", ExpensePayer.EMPLOYEE), graph.draft(receiptJpeg(seed = 2), ReceiptSource.CAMERA),
                graph.fence.epoch, expenses.state.value.generation)
            assertNotNull("The expense was not saved", expenses.state.value.lastSaved)
            graph.service.finish(graph.source.account(), a.partition(fixture), trip, reading("184610"))
            graph.assertSavedOnly(a, trips = 1, events = 1, finished = true)
            // With no route an attempt sends nothing and changes nothing.
            graph.delivery.dispatch(graph.source.account(), a.partition(fixture))
            graph.deliverAttachmentsOffline(a)
            assertEquals(0, graph.operationsSent.get() + graph.lostAnswers.get() + graph.sessionsSent.get() + graph.uploadsSent.get())
            graph.mark(2)
        } finally { graph.close() }
    }

    @Test
    fun stage3TheFieldDaySurvivesProcessDeathSealedToItsPartition(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(2)
            val (a, b) = fixture.organizations
            graph.assertSavedOnly(a, trips = 1, events = 1, finished = true)
            val photos = graph.photos(a)
            assertEquals(2, photos.size)
            for (photo in photos) assertEquals(photo.attachment.sha256, AttachmentPolicy.sha256(requireNotNull(graph.sealed(a, photo.attachment.documentId))))
            assertTrue(graph.trips(b).isEmpty())
            assertTrue(graph.photos(b).isEmpty())
            graph.delivery.dispatch(graph.source.account(), a.partition(fixture))
            graph.deliverAttachmentsOffline(a)
            assertEquals(0, graph.operationsSent.get() + graph.lostAnswers.get() + graph.sessionsSent.get() + graph.uploadsSent.get())
            graph.mark(3)
        } finally { graph.close() }
    }

    @Test
    fun stage4ALostAnswerAndATruncatedReceiptConvergeOnOneEffectEach(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(3)
            val (a, b) = fixture.organizations
            // The fixture commits the first batch and closes the connection before its answer: nothing is acknowledged.
            graph.delivery.dispatch(graph.source.deliveryAccount(), a.partition(fixture))
            assertEquals("The first intake answer was not lost", 1, graph.lostAnswers.get())
            graph.assertSavedOnly(a, trips = 1, events = 1, finished = true)
            val diagnostics = StringBuilder()
            var accepted = false
            for (attempt in 1..25) {
                Thread.sleep(3_000)
                try { graph.delivery.dispatch(graph.source.deliveryAccount(), a.partition(fixture)) }
                catch (failure: Exception) { diagnostics.append(failure.javaClass.simpleName).append("; ") }
                if (graph.everythingAccepted(a)) { accepted = true; break }
            }
            assertTrue("The field day did not converge after the lost answer: $diagnostics ${graph.statuses}", accepted)
            // The first transfer is truncated by the fixture (422) and stays sending; the retry stores both receipts.
            graph.deliverAttachments(a)
            assertTrue(graph.statuses.contains("upload:422"))
            var stored = false
            for (attempt in 1..15) {
                if (graph.photos(a).all { it.label == AttachmentLabel.VERIFIED }) { stored = true; break }
                Thread.sleep(3_000)
                graph.deliverAttachments(a)
            }
            assertTrue("The receipts were not stored: ${graph.statuses}", stored)
            assertTrue(graph.photos(a).all { graph.sealed(a, it.attachment.documentId) == null })
            val sent = graph.operationsSent.get() + graph.sessionsSent.get() + graph.uploadsSent.get()
            graph.delivery.dispatch(graph.source.deliveryAccount(), a.partition(fixture))
            graph.deliverAttachments(a)
            assertEquals("Accepted work or stored receipts were sent again", sent, graph.operationsSent.get() + graph.sessionsSent.get() + graph.uploadsSent.get())
            assertTrue(graph.trips(b).isEmpty())
        } finally {
            graph.cleanup()
            graph.close()
        }
    }

    private fun fixtureFile() = File(context.noBackupFilesDir, FIXTURE_NAME)

    private class Organization(val id: String, val member: String, val employee: String, val asset: String, val meter: String?, val segment: String?) {
        fun partition(fixture: SourceFixture) = EnterprisePartition.of(fixture.session.accountId, fixture.origin, id)
        fun context(fixture: SourceFixture) = EnterpriseContextSummary(partition(fixture), member, "Synthetic resilience", employee)
        override fun toString(): String = "ResilienceFixtureOrganization(redacted)"
    }
    private class SourceFixture(val origin: String, val certificate: String, val session: PortableAuth.Session, val organizations: List<Organization>) {
        override fun toString(): String = "ResilienceSourceFixture(redacted)"
    }

    private class Graph(private val context: Context, private val fixture: SourceFixture, seed: Boolean) : Closeable {
        val preferences = context.getSharedPreferences("enterprise-native-resilience", Context.MODE_PRIVATE)
        val fence = EnterpriseSessionFence()
        private val platformKeys = AndroidKeystoreKeys()
        private val sessions = EnterpriseFencedSessionStore(KeystoreSessionStore(preferences, platformKeys, SESSION_ALIAS), fence)
        private val accountKeys = KeystoreAccountKey(preferences, platformKeys, BACKUP_ALIAS)
        init {
            if (seed) {
                assertNull("Unexpected existing native resilience session", sessions.load())
                sessions.save(fixture.session)
            }
            val current = sessions.load() ?: throw AssertionError("Retained native resilience session is unavailable")
            assertTrue("Retained source changed ownership", current.accountId == fixture.session.accountId && current.familyId == fixture.session.familyId)
        }
        /** Answers actually received from the source; a lost answer is counted apart and never as a receipt. */
        val operationsSent = AtomicInteger()
        val lostAnswers = AtomicInteger()
        val sessionsSent = AtomicInteger()
        val uploadsSent = AtomicInteger()
        val statuses: MutableList<String> = java.util.Collections.synchronizedList(mutableListOf())
        private val http = fixtureHttp(fixture.certificate).newBuilder().addInterceptor { chain ->
            val path = chain.request().url.encodedPath
            val response = try { chain.proceed(chain.request()) } catch (lost: java.io.IOException) {
                if (path.endsWith("/sync/operations")) { lostAnswers.incrementAndGet(); statuses += "op:lost" }
                throw lost
            }
            response.also {
                when {
                    path.endsWith("/sync/operations") -> { operationsSent.incrementAndGet(); statuses += "op:${it.code}" }
                    path.endsWith("/sync/attachments") -> { sessionsSent.incrementAndGet(); statuses += "session:${it.code}" }
                    path.endsWith("/content/upload") -> { uploadsSent.incrementAndGet(); statuses += "upload:${it.code}" }
                }
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
        val persistence = ProtectedEnterprisePersistence(files, keys, fence, erasure,
            { sessions.load()?.let { EnterpriseLocalIdentity(it.accountId, fixture.origin, it.familyId) } }, clock::read, contact)
        val source = EnterpriseOperationSessionSource(fixture.origin, sessions, auth, fence, persistence::protectedEnrollment)
        val references = EnterpriseReferenceService(source, ProtectedReferenceCacheStore(persistence),
            EnterpriseReferenceHttpClient(http, source, signer), clock::read)
        private val queue = ProtectedEnterpriseQueue(persistence)
        private val hints = EnterpriseTripStartHints()
        // The product queue with the test's short backoff base, so the resend after the lost answer is due within seconds.
        val delivery = EnterpriseQueueService(source, queue, EnterpriseOperationHttpClient(http, source, signer, SyncLimits.DEFAULT),
            EnterpriseQueueRegistry(*EnterpriseTripProducers.all(hints), EnterpriseFuelingProducer(), EnterpriseExpenseProducer()), clock::read,
            entropy = { 0L }, retryPolicy = QueueRetryPolicy(2_000, 8_000))
        private val store = ProtectedTripStore(persistence)
        val service = EnterpriseTripService(source, delivery::save, store, hints, clock::read, { ZoneId.systemDefault() })
        val attachmentStore = ProtectedAttachmentStore(persistence)
        val attachments = EnterpriseAttachmentService(source, attachmentStore, EnterpriseAttachmentHttpClient(http, source, signer), clock::read,
            Instant::now, entropy = { 0L }, newId = { UUID.randomUUID().toString() }, enabled = true, retryPolicy = QueueRetryPolicy(2_000, 8_000))
        val receipts = EnterpriseReceipts(attachments, enabled = true)
        private val originals = AndroidReceiptOriginals(context)
        suspend fun draft(bytes: ByteArray, source: ReceiptSource): ReceiptDraft {
            val accepted = ReceiptIntake.inspect(bytes, source) as ReceiptInspection.Accepted
            return ReceiptDraft(accepted.original, ReceiptLegibility.assess(originals.sample(accepted.original.bytes())))
        }
        fun fuelingFlow() = FuelingFlow(source, EnterpriseFuelingService(source, delivery::save, ProtectedFuelingStore(persistence), { ZoneId.systemDefault() }),
            fence, receipts) { account, partition -> delivery.dispatch(account, partition) }
        fun expenseFlow() = ExpenseFlow(source, EnterpriseExpenseService(source, delivery::save, ProtectedExpenseStore(persistence), { ZoneId.systemDefault() }),
            fence, receipts) { account, partition -> delivery.dispatch(account, partition) }
        suspend fun photos(organization: Organization) = attachments.attachments(organization.partition(fixture))
        suspend fun deliverAttachments(organization: Organization) = attachments.deliver(source.deliveryAccount(), organization.partition(fixture))
        suspend fun deliverAttachmentsOffline(organization: Organization) {
            try { attachments.deliver(source.account(), organization.partition(fixture)) } catch (_: Exception) { }
        }
        suspend fun sealed(organization: Organization, document: String) = attachmentStore.bytes(source.attempt(source.account(), organization.partition(fixture)), document)
        suspend fun trips(organization: Organization): List<TripView> = service.trips(source.account(), organization.partition(fixture))

        /** Every record of the field day is "Salvo no aparelho": nothing was acknowledged. */
        suspend fun assertSavedOnly(organization: Organization, trips: Int, events: Int, finished: Boolean) {
            val views = trips(organization)
            assertEquals(trips, views.size)
            val view = views.single()
            assertEquals(events, view.events.size)
            assertEquals(finished, view.finish != null)
            assertTrue((listOf(view.start) + view.events + listOfNotNull(view.finish)).all { it.sync == EnterpriseSyncStatus.SAVED_ON_DEVICE })
            val fuelings = fuelingFlow().also { it.open(organization.context(fixture), fence.epoch) }.state.value.fuelings
            val expenses = expenseFlow().also { it.open(organization.context(fixture), fence.epoch) }.state.value.expenses
            assertEquals(1, fuelings.size); assertEquals(1, expenses.size)
            assertTrue(fuelings.all { it.sync == EnterpriseSyncStatus.SAVED_ON_DEVICE } && expenses.all { it.sync == EnterpriseSyncStatus.SAVED_ON_DEVICE })
        }

        suspend fun everythingAccepted(organization: Organization): Boolean {
            val view = trips(organization).singleOrNull() ?: return false
            val fuelings = fuelingFlow().also { it.open(organization.context(fixture), fence.epoch) }.state.value.fuelings
            val expenses = expenseFlow().also { it.open(organization.context(fixture), fence.epoch) }.state.value.expenses
            return (listOf(view.start) + view.events + listOfNotNull(view.finish)).all { it.sync == EnterpriseSyncStatus.ACCEPTED } &&
                fuelings.isNotEmpty() && fuelings.all { it.sync == EnterpriseSyncStatus.ACCEPTED } &&
                expenses.isNotEmpty() && expenses.all { it.sync == EnterpriseSyncStatus.ACCEPTED }
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

        fun cleanup() {
            fixture.organizations.forEach { assertTrue("Native resilience cleanup was incomplete", erasure.partitionRevoked(it.partition(fixture))) }
            val identityAlias = AndroidAnonymousDeviceKeys.aliasFor(AuthFlow.AuthState.Active(fixture.session), fixture.origin)
            KeyStore.getInstance("AndroidKeyStore").apply {
                load(null); deleteEntry(identityAlias); deleteEntry(SESSION_ALIAS); deleteEntry(BACKUP_ALIAS)
            }
            assertTrue(preferences.edit().clear().commit())
            val file = File(context.noBackupFilesDir, FIXTURE_NAME)
            assertTrue("Private native proof cleanup failed", !file.exists() || file.delete())
            assertFalse("Private native proof survived cleanup", file.exists())
        }

        override fun close() {
            http.connectionPool.evictAll()
            http.dispatcher.executorService.shutdown()
        }
    }

    companion object {
        private const val STAGE = "resilience_stage_process"
        private const val STAGE_NUMBER = "resilience_stage_number"
        private const val SESSION_ALIAS = "enterprise_native_resilience_session"
        private const val BACKUP_ALIAS = "enterprise_native_resilience_backup"
        const val FIXTURE_NAME = "enterprise-native-resilience-fixture.json"

        /** A real JPEG encoded on the device from a synthetic text-like receipt picture; never a camera photo. */
        fun receiptJpeg(seed: Int): ByteArray {
            val bitmap = android.graphics.Bitmap.createBitmap(640, 960, android.graphics.Bitmap.Config.ARGB_8888)
            val pixels = IntArray(640 * 960) { index ->
                val x = index % 640
                val y = index / 640
                val v = if (y % 24 < 8 && (x + seed * 7) % 48 < 36 && x in 40 until 600) 25 else 235
                android.graphics.Color.rgb(v, v, v)
            }
            bitmap.setPixels(pixels, 0, 640, 0, 0, 640, 960)
            return java.io.ByteArrayOutputStream().use { out ->
                check(bitmap.compress(android.graphics.Bitmap.CompressFormat.JPEG, 92, out)); out.toByteArray()
            }.also { bitmap.recycle() }
        }

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
        } catch (_: Exception) { throw AssertionError("Disposable resilience fixture is invalid") }

        private fun fixtureHttp(pem: String): OkHttpClient = try {
            val certificate = CertificateFactory.getInstance("X.509").generateCertificate(pem.byteInputStream())
            val anchors = KeyStore.getInstance(KeyStore.getDefaultType()).apply { load(null); setCertificateEntry("disposable-resilience", certificate) }
            val trust = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply { init(anchors) }
                .trustManagers.single() as X509TrustManager
            val tls = SSLContext.getInstance("TLS").apply { init(null, arrayOf(trust), null) }
            OkHttpClient.Builder().sslSocketFactory(tls.socketFactory, trust).dns(object : Dns {
                override fun lookup(hostname: String): List<InetAddress> {
                    check(hostname == "api.anpfuel.example.invalid") { "Native resilience test refused an unrelated host" }
                    return listOf(InetAddress.getByAddress(byteArrayOf(127, 0, 0, 1)))
                }
            }).followRedirects(false).followSslRedirects(false).retryOnConnectionFailure(false).build()
        } catch (_: Exception) { throw AssertionError("Disposable resilience trust anchor is invalid") }
    }
}
