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
import com.anpfuel.application.enterprise.expense.EnterpriseExpenseProducer
import com.anpfuel.application.enterprise.expense.EnterpriseExpenseService
import com.anpfuel.application.enterprise.expense.ExpenseFlow
import com.anpfuel.application.enterprise.expense.ExpenseForm
import com.anpfuel.application.enterprise.expense.ExpensePayer
import com.anpfuel.data.enterprise.expense.ProtectedExpenseStore
import com.anpfuel.domain.enterprise.expense.ExpenseReviewLine
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
 * Opt-in four-process EMP-EXP-004 acceptance against the private ordinary PostGIS/HTTPS/MinIO receipt fixture
 * (orgsync_receipt_native_fixture_test.go), with the product's real protected store, queue with the `expense.submit`
 * producer, the [ExpenseFlow] owner, the EMP-DOC-004 capture path, the attachment service, signer and HTTP clients.
 * Stage 1 is online (custody and the reference pull); the driver then force-stops this target and removes the route;
 * stage 2 records two expenses offline through the flow, one with its receipt in the same save and one whose receipt
 * is added later; stage 3 proves both survive the process death sealed to their partition with their exact centavos;
 * the route returns for stage 4 (both expenses accepted, one receipt stored, the other's transfer refused while its
 * expense stays accepted, then stored on retry; nothing is sent twice).
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseExpenseNativeDeviceTest {
    private val context: Context get() = InstrumentationRegistry.getInstrumentation().targetContext

    @Test
    fun stage1OnlineCustodyAndReferencePull(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = true)
        try {
            assertEquals("Unexpected previous native expense stage", 0, graph.preferences.getInt(STAGE_NUMBER, 0))
            graph.establishCustody()
            val (a, _) = fixture.organizations
            val cache = requireNotNull(graph.references.refresh(graph.source.deliveryAccount(), a.partition(fixture)).cache)
            assertTrue(cache.assets.any { it.id == a.asset })
            graph.mark(1)
        } finally { graph.close() }
    }

    @Test
    fun stage2OfflineExpensesAreSavedWithTheirReceiptsThroughTheFlow(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(1)
            val (a, b) = fixture.organizations
            val cached = requireNotNull(graph.references.cached(graph.source.account(), a.partition(fixture)))
            val snapshot = MyAssets.present(cached, graph.clock.read())
            val choice = requireNotNull(MyAssets.choose(a.id, snapshot, a.asset, warningAccepted = false))
            val flow = graph.expenseFlow()
            flow.open(a.context(fixture), graph.fence.epoch)
            // A toll of R$ 12,50 paid by the employee, all of it for the company, with its receipt in the same save.
            val draft = graph.draft(receiptJpeg(dark = false), ReceiptSource.CAMERA)
            flow.record(choice, ExpenseForm("TOLL", "12,50", "12,50", ExpensePayer.EMPLOYEE), draft, graph.fence.epoch, flow.state.value.generation)
            val first = flow.state.value
            assertNotNull("The expense was not saved: ${first.saveFailed} ${first.captureInvalid} ${first.error}", first.lastSaved)
            assertEquals(first.lastSaved, first.lastAttached)
            // A value with a third decimal saves nothing (B-BR-EMP-519).
            flow.record(choice, ExpenseForm("PARKING", "8,005", "8", ExpensePayer.COMPANY), null, graph.fence.epoch, flow.state.value.generation)
            assertTrue(flow.state.value.captureInvalid)
            assertEquals(1, flow.state.value.expenses.size)
            // Parking of R$ 8,00 paid by the company, of which R$ 6,40 for the company; its receipt is added later from the detail.
            flow.record(choice, ExpenseForm("PARKING", "8", "6,40", ExpensePayer.COMPANY), null, graph.fence.epoch, flow.state.value.generation)
            val later = flow.state.value.expenses.first { it.fact.category == "PARKING" }
            flow.attach(later.fact.expenseId, graph.draft(receiptJpeg(dark = false, seed = 2), ReceiptSource.IMPORT), graph.fence.epoch, flow.state.value.generation)
            val state = flow.state.value
            assertEquals(2, state.expenses.size)
            assertTrue(state.expenses.all { it.sync == EnterpriseSyncStatus.SAVED_ON_DEVICE && it.review == ExpenseReviewLine.NOT_SENT })
            val photos = state.attachments.values.flatten()
            assertEquals(2, photos.size)
            assertTrue(photos.all { it.label == AttachmentLabel.SAVED_ON_DEVICE && it.attachment.target.kind == DocumentTargetKind.EXPENSE })
            assertEquals(state.expenses.map { it.fact.expenseId }.toSet(), photos.map { it.attachment.target.id }.toSet())
            // BR-OFFL-003: organization B lists nothing and cannot own A's expense receipt.
            val other = graph.expenseFlow()
            other.open(b.context(fixture), graph.fence.epoch)
            assertTrue(other.state.value.expenses.isEmpty())
            val foreign = runCatching { graph.receipts.save(b.partition(fixture), later.fact.operationId,
                AttachmentTarget(DocumentTargetKind.EXPENSE, later.fact.expenseId), draft) }.exceptionOrNull()
            assertEquals(AttachmentLocalFailure.UNKNOWN_OPERATION, (foreign as ReceiptSaveRefused).failure)
            graph.delivery.dispatch(graph.source.account(), a.partition(fixture))
            graph.deliverAttachmentsOffline(a)
            assertEquals(0, graph.operationsSent.get() + graph.sessionsSent.get() + graph.uploadsSent.get())
            graph.mark(2)
        } finally { graph.close() }
    }

    @Test
    fun stage3ExpensesAndReceiptsSurviveProcessDeathSealedToTheirPartition(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(2)
            val (a, b) = fixture.organizations
            val flow = graph.expenseFlow()
            flow.open(a.context(fixture), graph.fence.epoch)
            val expenses = flow.state.value.expenses
            assertEquals(setOf(1_250L to 1_250L, 800L to 640L), expenses.map { it.fact.paidCentavos to it.fact.companyCentavos }.toSet())
            assertEquals(setOf("TOLL" to "EMPLOYEE", "PARKING" to "COMPANY"), expenses.map { it.fact.category to it.fact.payer }.toSet())
            assertTrue(expenses.all { it.sync == EnterpriseSyncStatus.SAVED_ON_DEVICE })
            val photos = graph.photos(a)
            assertEquals(2, photos.size)
            for (photo in photos) assertEquals(photo.attachment.sha256, AttachmentPolicy.sha256(requireNotNull(graph.sealed(a, photo.attachment.documentId))))
            val other = graph.expenseFlow()
            other.open(b.context(fixture), graph.fence.epoch)
            assertTrue(other.state.value.expenses.isEmpty())
            assertTrue(graph.photos(b).isEmpty())
            graph.assertProtectedSurfaces(expenses.flatMap { listOf(it.fact.expenseId, it.fact.operationId) } +
                photos.flatMap { listOf(it.attachment.documentId, it.attachment.sha256) } + listOf("expense.submit", "TOLL", "PARKING", "EXPENSE", "JFIF"))
            graph.delivery.dispatch(graph.source.account(), a.partition(fixture))
            graph.deliverAttachmentsOffline(a)
            assertEquals(0, graph.operationsSent.get() + graph.sessionsSent.get() + graph.uploadsSent.get())
            graph.mark(3)
        } finally { graph.close() }
    }

    @Test
    fun stage4ExpensesAreAcceptedWhileARefusedReceiptTransferStaysSeparateThenBothAreStored(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            graph.requireStage(3)
            val (a, _) = fixture.organizations
            val flow = graph.expenseFlow()
            flow.open(a.context(fixture), graph.fence.epoch)
            val diagnostics = StringBuilder()
            for (attempt in 1..20) {
                try { graph.delivery.dispatch(graph.source.deliveryAccount(), a.partition(fixture)) }
                catch (failure: Exception) { diagnostics.append(failure.javaClass.simpleName).append("; ") }
                flow.reload(graph.fence.epoch, flow.state.value.generation)
                if (flow.state.value.expenses.all { it.sync == EnterpriseSyncStatus.ACCEPTED }) break
                Thread.sleep(3_000)
            }
            val accepted = flow.state.value.expenses
            assertTrue("The expenses were not accepted: $diagnostics ${graph.statuses}", accepted.size == 2 && accepted.all { it.sync == EnterpriseSyncStatus.ACCEPTED })
            assertTrue(accepted.all { it.review == ExpenseReviewLine.AWAITING_APPROVAL })
            // The plan forwards the first transfer and truncates the second: one receipt stored, the other refused (422)
            // and still sending while its expense stays accepted (B-BR-EMP-520).
            graph.deliverAttachments(a)
            assertEquals(listOf("upload:200", "upload:422"), graph.statuses.filter { it.startsWith("upload:") })
            flow.reload(graph.fence.epoch, flow.state.value.generation)
            val labels = flow.state.value.attachments.values.flatten().map { it.label }.sorted()
            assertEquals(listOf(AttachmentLabel.SENDING, AttachmentLabel.VERIFIED).sorted(), labels)
            assertTrue(flow.state.value.expenses.all { it.sync == EnterpriseSyncStatus.ACCEPTED })
            var stored = false
            for (attempt in 1..15) {
                Thread.sleep(3_000)
                graph.deliverAttachments(a)
                if (graph.photos(a).all { it.label == AttachmentLabel.VERIFIED }) { stored = true; break }
            }
            assertTrue("The receipts were not stored: ${graph.statuses}", stored)
            assertTrue(graph.photos(a).all { graph.sealed(a, it.attachment.documentId) == null })
            val sent = graph.operationsSent.get() + graph.sessionsSent.get() + graph.uploadsSent.get()
            graph.delivery.dispatch(graph.source.deliveryAccount(), a.partition(fixture))
            graph.deliverAttachments(a)
            assertEquals("Accepted work or stored receipts were sent again", sent, graph.operationsSent.get() + graph.sessionsSent.get() + graph.uploadsSent.get())
        } finally {
            graph.cleanup()
            graph.close()
        }
    }

    private fun fixtureFile() = File(context.noBackupFilesDir, "enterprise-native-expense-fixture.json")

    private class Organization(val id: String, val member: String, val employee: String, val asset: String, val meter: String?, val segment: String?) {
        fun partition(fixture: SourceFixture) = EnterprisePartition.of(fixture.session.accountId, fixture.origin, id)
        fun context(fixture: SourceFixture) = EnterpriseContextSummary(partition(fixture), member, "Synthetic trips", employee)
        override fun toString(): String = "TripFixtureOrganization(redacted)"
    }
    private class SourceFixture(val origin: String, val certificate: String, val session: PortableAuth.Session, val organizations: List<Organization>) {
        override fun toString(): String = "TripSourceFixture(redacted)"
    }

    private class Graph(private val context: Context, private val fixture: SourceFixture, seed: Boolean) : Closeable {
        val preferences = context.getSharedPreferences("enterprise-native-expense", Context.MODE_PRIVATE)
        val fence = EnterpriseSessionFence()
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
        val persistence = ProtectedEnterprisePersistence(files, keys, fence, erasure,
            { sessions.load()?.let { EnterpriseLocalIdentity(it.accountId, fixture.origin, it.familyId) } }, clock::read, contact)
        val source = EnterpriseOperationSessionSource(fixture.origin, sessions, auth, fence, persistence::protectedEnrollment)
        val references = EnterpriseReferenceService(source, ProtectedReferenceCacheStore(persistence),
            EnterpriseReferenceHttpClient(http, source, signer), clock::read)
        private val queue = ProtectedEnterpriseQueue(persistence)
        private val hints = EnterpriseTripStartHints()
        val delivery = EnterpriseQueueService(source, queue, EnterpriseOperationHttpClient(http, source, signer, SyncLimits.DEFAULT),
            EnterpriseQueueRegistry(*EnterpriseTripProducers.all(hints), EnterpriseExpenseProducer()), clock::read, entropy = { 0L })
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
        /** The product's expense owner over this graph's queue, store and receipts. */
        fun expenseFlow() = ExpenseFlow(source, EnterpriseExpenseService(source, delivery::save, ProtectedExpenseStore(persistence), { ZoneId.systemDefault() }),
            fence, receipts) { account, partition -> delivery.dispatch(account, partition) }
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
            fixture.organizations.forEach { assertTrue("Native expense cleanup was incomplete", erasure.partitionRevoked(it.partition(fixture))) }
            val identityAlias = AndroidAnonymousDeviceKeys.aliasFor(AuthFlow.AuthState.Active(fixture.session), fixture.origin)
            KeyStore.getInstance("AndroidKeyStore").apply {
                load(null); deleteEntry(identityAlias); deleteEntry(SESSION_ALIAS); deleteEntry(BACKUP_ALIAS)
            }
            assertTrue(preferences.edit().clear().commit())
            for (name in listOf("enterprise-native-expense-fixture.json", "enterprise-native-trip-second-started")) {
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
        private const val STAGE = "expense_stage_process"
        private const val STAGE_NUMBER = "expense_stage_number"
        private const val SESSION_ALIAS = "enterprise_native_expense_session"
        private const val BACKUP_ALIAS = "enterprise_native_expense_backup"
        const val UPLOADING_MARKER = "enterprise-native-expense-uploading"

        /** A real JPEG encoded on the device from a synthetic text-like receipt picture; never a camera photo. */
        fun receiptJpeg(dark: Boolean, seed: Int = 1): ByteArray {
            val bitmap = android.graphics.Bitmap.createBitmap(640, 960, android.graphics.Bitmap.Config.ARGB_8888)
            val paper = if (dark) 40 else 235
            val ink = if (dark) 10 else 25
            val pixels = IntArray(640 * 960) { index ->
                val x = index % 640
                val y = index / 640
                val v = if (y % 24 < 8 && (x + seed * 7) % 48 < 36 && x in 40 until 600) ink else paper
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
        } catch (_: Exception) { throw AssertionError("Disposable expense fixture is invalid") }

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
