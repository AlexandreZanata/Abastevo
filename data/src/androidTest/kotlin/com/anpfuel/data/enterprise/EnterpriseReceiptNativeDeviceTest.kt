package com.anpfuel.data.enterprise

import android.content.Context
import android.os.Process
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.attachment.EnterpriseAttachmentService
import com.anpfuel.application.enterprise.attachment.EnterpriseReceipts
import com.anpfuel.application.enterprise.attachment.ReceiptDraft
import com.anpfuel.application.enterprise.attachment.ReceiptSaveRefused
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
import com.anpfuel.data.enterprise.attachment.AndroidReceiptOriginals
import com.anpfuel.data.enterprise.attachment.EnterpriseAttachmentHttpClient
import com.anpfuel.data.enterprise.attachment.ProtectedAttachmentStore
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
import com.anpfuel.domain.enterprise.attachment.AttachmentLabel
import com.anpfuel.domain.enterprise.attachment.AttachmentLocalFailure
import com.anpfuel.domain.enterprise.attachment.LegibilityIssue
import com.anpfuel.domain.enterprise.attachment.ReceiptInspection
import com.anpfuel.domain.enterprise.attachment.ReceiptIntake
import com.anpfuel.domain.enterprise.attachment.ReceiptLegibility
import com.anpfuel.domain.enterprise.attachment.ReceiptRefusal
import com.anpfuel.domain.enterprise.attachment.ReceiptSource
import com.anpfuel.domain.enterprise.attachment.AttachmentPolicy
import com.anpfuel.domain.enterprise.attachment.AttachmentState
import com.anpfuel.domain.enterprise.attachment.AttachmentTarget
import com.anpfuel.domain.enterprise.attachment.AttachmentView
import com.anpfuel.domain.enterprise.document.DocumentTargetKind
import com.anpfuel.domain.enterprise.fleet.MyAssets
import com.anpfuel.domain.enterprise.meter.MeterEntryForm
import com.anpfuel.domain.enterprise.meter.MeterKind
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import com.anpfuel.domain.enterprise.sync.QueueRetryPolicy
import com.anpfuel.domain.enterprise.sync.SyncLimits
import com.anpfuel.domain.enterprise.trip.TripAssetLabel
import com.anpfuel.domain.enterprise.trip.TripEvidence
import com.anpfuel.domain.enterprise.trip.TripOperationFact
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
 * Opt-in four-process EMP-DOC-004 acceptance against the private ordinary PostGIS/HTTPS/MinIO receipt fixture
 * (orgsync_receipt_native_fixture_test.go), with the product's real protected store, queue, trip producers, attachment
 * service, the DOC-004 capture path (content intake, the Android decoder's legibility sample, [EnterpriseReceipts]),
 * signer and HTTP clients. Stage 1 is online (custody and the reference pull); the driver then force-stops this target
 * and removes the route; stage 2 starts a trip and captures a synthetic odometer photo offline (a PNG import and a
 * foreign organization are refused); stage 3 proves it survives the process death sealed to its partition; the route
 * returns for stage 4 (the start is accepted, a truncated transfer is refused while the record stays accepted, and the
 * photo is stored on retry).
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseReceiptNativeDeviceTest {
    private val context: Context get() = InstrumentationRegistry.getInstrumentation().targetContext

    @Test
    fun stage1OnlineCustodyAndReferencePull(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = true)
        try {
            assertEquals("Unexpected previous native receipt stage", 0, graph.preferences.getInt(STAGE_NUMBER, 0))
            graph.establishCustody()
            val (a, _) = fixture.organizations
            val cache = requireNotNull(graph.references.refresh(graph.source.deliveryAccount(), a.partition(fixture)).cache)
            assertEquals(listOf(a.meter!!), cache.assets.single { it.id == a.asset }.meters.map { it.id })
            graph.mark(1)
        } finally { graph.close() }
    }

    @Test
    fun stage2OfflineCaptureFollowsTheDoc004PathAndRefusesWhatItMust(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(1)
            val (a, b) = fixture.organizations
            val cached = requireNotNull(graph.references.cached(graph.source.account(), a.partition(fixture)))
            val snapshot = MyAssets.present(cached, graph.clock.read())
            val choice = requireNotNull(MyAssets.choose(a.id, snapshot, a.asset, warningAccepted = false))
            val asset = snapshot.assets.single { it.asset.id == a.asset }.asset
            val meter = choice.meters.single()
            graph.service.start(graph.source.account(), a.context(fixture), choice, TripAssetLabel(asset.name, asset.fleetNumber, asset.plate),
                TripCapture.Reading(TripMeterChoice(meter.id, meter.segmentId, MeterKind.ODOMETER, meter.modulus), MeterEntryForm.KM, "184320"))
            val trip = graph.trips(a).single()
            val target = AttachmentTarget(DocumentTargetKind.METER_READING, departureReading(trip))
            // A camera-like original: a real JPEG encoded on the device from a synthetic text-like picture.
            val photo = receiptJpeg(dark = false)
            val draft = graph.draft(photo, ReceiptSource.CAMERA)
            assertTrue("A sharp synthetic receipt must read as sharp: ${draft.hint}", draft.hint.looksSharp)
            val saved = graph.receipts.save(a.partition(fixture), trip.trip.startOperationId, target, draft)
            assertEquals(listOf(AttachmentLabel.SAVED_ON_DEVICE), graph.photos(a).map { it.label })
            assertEquals(EnterpriseSyncStatus.SAVED_ON_DEVICE, graph.trips(a).single().start.sync)
            // The Android decoder sees a dark original as dark; the hint never blocks and is never stored.
            val dark = graph.draft(receiptJpeg(dark = true), ReceiptSource.IMPORT)
            assertTrue(LegibilityIssue.TOO_DARK in dark.hint.issues)
            // B-BR-EMP-451: a PNG import is refused by content and never listed.
            assertEquals(ReceiptRefusal.UNSUPPORTED, (ReceiptIntake.inspect(syntheticPng(), ReceiptSource.IMPORT) as ReceiptInspection.Refused).refusal)
            // BR-OFFL-003: A's record cannot own a photo in B's partition.
            val foreign = runCatching { graph.receipts.save(b.partition(fixture), trip.trip.startOperationId, target, draft) }.exceptionOrNull()
            assertEquals(AttachmentLocalFailure.UNKNOWN_OPERATION, (foreign as ReceiptSaveRefused).failure)
            assertEquals(1, graph.photos(a).size)
            assertTrue(graph.photos(b).isEmpty())
            graph.delivery.dispatch(graph.source.account(), a.partition(fixture))
            graph.deliverAttachmentsOffline(a)
            assertEquals(0, graph.operationsSent.get() + graph.sessionsSent.get() + graph.uploadsSent.get())
            graph.preferences.edit().putString(PHOTO_A, saved.documentId).putString(PHOTO_SHA, saved.sha256).commit()
            graph.mark(2)
        } finally { graph.close() }
    }

    @Test
    fun stage3TheSavedOriginalSurvivesProcessDeathSealedToItsPartition(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(2)
            val (a, b) = fixture.organizations
            val photo = graph.photos(a).single()
            assertEquals(graph.preferences.getString(PHOTO_A, null), photo.attachment.documentId)
            assertEquals(AttachmentLabel.SAVED_ON_DEVICE, photo.label)
            val sealed = requireNotNull(graph.sealed(a, photo.attachment.documentId))
            // The original is kept exactly as captured (B-BR-EMP-451).
            assertArrayEquals(receiptJpeg(dark = false), sealed)
            assertEquals(graph.preferences.getString(PHOTO_SHA, null), AttachmentPolicy.sha256(sealed))
            assertTrue(graph.photos(b).isEmpty())
            graph.assertProtectedSurfaces(listOf(photo.attachment.documentId, photo.attachment.sha256, photo.attachment.target.id,
                "attachment-book", "METER_READING", "JFIF"))
            graph.deliverAttachmentsOffline(a)
            assertEquals(0, graph.operationsSent.get() + graph.sessionsSent.get() + graph.uploadsSent.get())
            graph.mark(3)
        } finally { graph.close() }
    }

    @Test
    fun stage4TheRecordIsAcceptedWhileARefusedTransferStaysSeparateThenThePhotoIsStored(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(3)
            val (a, _) = fixture.organizations
            val diagnostics = StringBuilder()
            var start: TripView? = null
            for (attempt in 1..20) {
                try { graph.delivery.dispatch(graph.source.deliveryAccount(), a.partition(fixture)) }
                catch (failure: Exception) { diagnostics.append(failure.javaClass.simpleName).append("; ") }
                val view = graph.trips(a).single()
                if (view.start.sync == EnterpriseSyncStatus.ACCEPTED) { start = view; break }
                Thread.sleep(3_000)
            }
            requireNotNull(start) { "The trip start was not accepted: $diagnostics" }
            // The fixture truncates the first transfer: the record stays accepted, the photo is never stored by it.
            val first = graph.deliverAttachments(a)
            assertEquals(0, first.stored)
            assertEquals(listOf("upload:422"), graph.statuses.filter { it.startsWith("upload:") })
            assertEquals(EnterpriseSyncStatus.ACCEPTED, graph.trips(a).single().start.sync)
            assertEquals(AttachmentLabel.SENDING, graph.photos(a).single().label)
            var stored = false
            for (attempt in 1..15) {
                Thread.sleep(3_000)
                graph.deliverAttachments(a)
                if (graph.photos(a).single().label == AttachmentLabel.VERIFIED) { stored = true; break }
            }
            assertTrue("The photo was not stored: ${graph.statuses}", stored)
            assertNull("Stored bytes stayed on the device", graph.sealed(a, graph.photos(a).single().attachment.documentId))
            val sent = graph.sessionsSent.get() + graph.uploadsSent.get()
            graph.deliverAttachments(a)
            assertEquals("A stored photo was sent again", sent, graph.sessionsSent.get() + graph.uploadsSent.get())
        } finally {
            graph.cleanup()
            graph.close()
        }
    }

    private fun departureReading(view: TripView): String =
        ((view.start.fact as TripOperationFact.Start).evidence as TripEvidence.Reading).readingId

    private fun fixtureFile() = File(context.noBackupFilesDir, "enterprise-native-receipt-fixture.json")

    private class Organization(val id: String, val member: String, val employee: String, val asset: String, val meter: String?, val segment: String?) {
        fun partition(fixture: SourceFixture) = EnterprisePartition.of(fixture.session.accountId, fixture.origin, id)
        fun context(fixture: SourceFixture) = EnterpriseContextSummary(partition(fixture), member, "Synthetic trips", employee)
        override fun toString(): String = "TripFixtureOrganization(redacted)"
    }
    private class SourceFixture(val origin: String, val certificate: String, val session: PortableAuth.Session, val organizations: List<Organization>) {
        override fun toString(): String = "TripSourceFixture(redacted)"
    }

    private class Graph(private val context: Context, private val fixture: SourceFixture, seed: Boolean) : Closeable {
        val preferences = context.getSharedPreferences("enterprise-native-receipt", Context.MODE_PRIVATE)
        private val fence = EnterpriseSessionFence()
        private val platformKeys = AndroidKeystoreKeys()
        private val sessions = EnterpriseFencedSessionStore(KeystoreSessionStore(preferences, platformKeys, SESSION_ALIAS), fence)
        private val accountKeys = KeystoreAccountKey(preferences, platformKeys, BACKUP_ALIAS)
        init {
            if (seed) {
                assertNull("Unexpected existing native attachment session", sessions.load())
                sessions.save(fixture.session)
            }
            val current = sessions.load() ?: throw AssertionError("Retained native attachment session is unavailable")
            // The retained session is the fixture's own family: a refresh never meets another family (FLEET-013 incident).
            assertTrue("Retained source changed ownership", current.accountId == fixture.session.accountId && current.familyId == fixture.session.familyId)
        }
        /** Answers actually received from the source (a refused connection is never counted). */
        val operationsSent = AtomicInteger()
        val sessionsSent = AtomicInteger()
        val uploadsSent = AtomicInteger()
        val statuses: MutableList<String> = java.util.Collections.synchronizedList(mutableListOf())
        private val http = fixtureHttp(fixture.certificate).newBuilder().addInterceptor { chain ->
            val path = chain.request().url.encodedPath
            if (path.endsWith("/content/upload")) java.io.File(context.noBackupFilesDir, UPLOADING_MARKER).writeText("1")
            chain.proceed(chain.request()).also {
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
        val attachmentStore = ProtectedAttachmentStore(persistence)
        // The product service with the test's short backoff base, so a refused transfer is due again within seconds.
        val attachments = EnterpriseAttachmentService(source, attachmentStore, EnterpriseAttachmentHttpClient(http, source, signer), clock::read,
            Instant::now, entropy = { 0L }, newId = { UUID.randomUUID().toString() }, enabled = true, retryPolicy = QueueRetryPolicy(2_000, 8_000))
        val receipts = EnterpriseReceipts(attachments, enabled = true)
        private val originals = AndroidReceiptOriginals(context)
        /** The DOC-004 path: content intake, then the Android decoder's sample for the hint. */
        suspend fun draft(bytes: ByteArray, source: com.anpfuel.domain.enterprise.attachment.ReceiptSource): ReceiptDraft {
            val accepted = ReceiptIntake.inspect(bytes, source) as ReceiptInspection.Accepted
            return ReceiptDraft(accepted.original, ReceiptLegibility.assess(originals.sample(accepted.original.bytes())))
        }
        suspend fun photos(organization: Organization) = attachments.attachments(organization.partition(fixture))
        suspend fun deliverAttachments(organization: Organization) = attachments.deliver(source.deliveryAccount(), organization.partition(fixture))
        suspend fun sealed(organization: Organization, document: String) = attachmentStore.bytes(source.attempt(source.account(), organization.partition(fixture)), document)

        suspend fun trips(organization: Organization): List<TripView> = service.trips(source.account(), organization.partition(fixture))
        /** With no route an attempt fails at the transport and keeps everything pending. */
        suspend fun deliverAttachmentsOffline(organization: Organization) {
            try { attachments.deliver(source.account(), organization.partition(fixture)) } catch (_: Exception) { }
        }
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

        /** No attachment byte, digest, identifier or credential appears outside encryption. */
        fun assertProtectedSurfaces(secrets: List<String>) {
            for (organization in fixture.organizations) {
                val token = EnterpriseVaultCodec.token(organization.partition(fixture))
                for (generation in files.generations(token)) for (descriptor in listOf(false, true)) {
                    val text = String(files.readGeneration(token, generation, descriptor) ?: continue, Charsets.ISO_8859_1)
                    for (secret in secrets + fixture.session.accountId + fixture.session.accessToken)
                        assertFalse("An attachment value appeared outside encryption", text.contains(secret))
                }
            }
        }

        fun cleanup() {
            fixture.organizations.forEach { assertTrue("Native receipt cleanup was incomplete", erasure.partitionRevoked(it.partition(fixture))) }
            val identityAlias = AndroidAnonymousDeviceKeys.aliasFor(AuthFlow.AuthState.Active(fixture.session), fixture.origin)
            KeyStore.getInstance("AndroidKeyStore").apply {
                load(null); deleteEntry(identityAlias); deleteEntry(SESSION_ALIAS); deleteEntry(BACKUP_ALIAS)
            }
            assertTrue(preferences.edit().clear().commit())
            for (name in listOf("enterprise-native-receipt-fixture.json", "enterprise-native-trip-second-started")) {
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
        private const val STAGE = "receipt_stage_process"
        private const val STAGE_NUMBER = "receipt_stage_number"
        private const val PHOTO_A = "receipt_a"
        private const val PHOTO_SHA = "receipt_a_sha"
        private const val SESSION_ALIAS = "enterprise_native_receipt_session"
        private const val BACKUP_ALIAS = "enterprise_native_receipt_backup"
        const val UPLOADING_MARKER = "enterprise-native-receipt-uploading"

        /** A real JPEG encoded on the device from a synthetic text-like receipt picture; never a camera photo. */
        fun receiptJpeg(dark: Boolean): ByteArray {
            val bitmap = android.graphics.Bitmap.createBitmap(640, 960, android.graphics.Bitmap.Config.ARGB_8888)
            val paper = if (dark) 40 else 235
            val ink = if (dark) 10 else 25
            val pixels = IntArray(640 * 960) { index ->
                val x = index % 640
                val y = index / 640
                val v = if (y % 24 < 8 && x % 48 < 36 && x in 40 until 600) ink else paper
                android.graphics.Color.rgb(v, v, v)
            }
            bitmap.setPixels(pixels, 0, 640, 0, 0, 640, 960)
            return java.io.ByteArrayOutputStream().use { out ->
                check(bitmap.compress(android.graphics.Bitmap.CompressFormat.JPEG, 92, out)); out.toByteArray()
            }.also { bitmap.recycle() }
        }

        /** A real PNG of the same kind of picture: refused by content (B-BR-EMP-451). */
        fun syntheticPng(): ByteArray {
            val bitmap = android.graphics.Bitmap.createBitmap(64, 64, android.graphics.Bitmap.Config.ARGB_8888)
            bitmap.eraseColor(android.graphics.Color.WHITE)
            return java.io.ByteArrayOutputStream().use { out ->
                check(bitmap.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, out)); out.toByteArray()
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
        } catch (_: Exception) { throw AssertionError("Disposable receipt fixture is invalid") }

        private fun fixtureHttp(pem: String): OkHttpClient = try {
            val certificate = CertificateFactory.getInstance("X.509").generateCertificate(pem.byteInputStream())
            val anchors = KeyStore.getInstance(KeyStore.getDefaultType()).apply { load(null); setCertificateEntry("disposable-receipt", certificate) }
            val trust = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply { init(anchors) }
                .trustManagers.single() as X509TrustManager
            val tls = SSLContext.getInstance("TLS").apply { init(null, arrayOf(trust), null) }
            OkHttpClient.Builder().sslSocketFactory(tls.socketFactory, trust).dns(object : Dns {
                override fun lookup(hostname: String): List<InetAddress> {
                    check(hostname == "api.anpfuel.example.invalid") { "Native receipt test refused an unrelated host" }
                    return listOf(InetAddress.getByAddress(byteArrayOf(127, 0, 0, 1)))
                }
            }).followRedirects(false).followSslRedirects(false).retryOnConnectionFailure(false).build()
        } catch (_: Exception) { throw AssertionError("Disposable receipt trust anchor is invalid") }
    }
}
