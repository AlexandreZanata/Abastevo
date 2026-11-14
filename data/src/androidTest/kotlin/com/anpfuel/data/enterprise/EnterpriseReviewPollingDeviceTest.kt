package com.anpfuel.data.enterprise

import android.content.Context
import android.os.Process
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.work.Data
import androidx.work.ListenableWorker
import androidx.work.WorkerFactory
import androidx.work.WorkerParameters
import androidx.work.testing.TestListenableWorkerBuilder
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.application.enterprise.sync.EnterpriseDeliveryActivity
import com.anpfuel.application.portable.*
import com.anpfuel.data.di.EnterpriseModule
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseSyncStatusStore
import com.anpfuel.data.local.auth.AccountHttpApi
import com.anpfuel.data.local.auth.AndroidAnonymousDeviceKeys
import com.anpfuel.data.local.auth.AndroidKeystoreKeys
import com.anpfuel.data.local.auth.KeystoreAccountKey
import com.anpfuel.data.local.auth.KeystoreSessionStore
import com.anpfuel.data.remote.AnonymousProofHttpClient
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.PhotoProofTransport
import com.anpfuel.data.worker.EnterpriseOperationWorker
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.sync.*
import com.anpfuel.domain.portable.PortableAuth
import java.io.Closeable
import java.io.File
import java.net.InetAddress
import java.security.KeyStore
import java.security.MessageDigest
import java.security.cert.CertificateFactory
import java.time.Instant
import java.util.UUID
import java.util.concurrent.atomic.AtomicInteger
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManagerFactory
import javax.net.ssl.X509TrustManager
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import okhttp3.Dns
import okhttp3.OkHttpClient
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Opt-in two-process review acceptance against the private ordinary PostGIS/HTTPS
 * Meter fixture (orgsync_review_native_fixture_test.go). Run the seed method, wait
 * for the fixture's actual reviewer decisions, force-stop only this target without
 * clearing data, then run the recovery method. No test producer enters the product.
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseReviewPollingDeviceTest {
    private val context: Context get() = InstrumentationRegistry.getInstrumentation().targetContext

    @Test
    fun seedHeldReviewsAndObserveThemUnresolved(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = true)
        try {
            assertEquals("Unexpected previous native review stage", 0, graph.preferences.getInt(STAGE_PID, 0))
            graph.establishCustody()
            for (organization in fixture.organizations) {
                val operation = graph.operation(organization)
                assertTrue(graph.service.save(organization.partition(fixture), operation))
                val delivered = graph.service.dispatch(organization.partition(fixture))
                assertEquals("Held work must still receive one final REVIEW receipt", 1, delivered.completed)
                val entry = graph.entry(organization)
                val ack = entry.acknowledgement ?: throw AssertionError("REVIEW receipt was not retained")
                assertEquals(SyncResult.ACCEPTED_FOR_REVIEW, ack.result)
                assertEquals("PERMISSION_CHANGED", ack.code)
                assertNotNull(ack.reviewItemId)
                assertTrue(entry.unresolvedReview && entry.reviewRetry.initial && entry.resolution == null)
                graph.preferences.edit().putString(ACK_PREFIX + organization.id, hex(ack.canonicalBytes)).commit()
            }
            assertEquals(2, graph.operationRequests.get())
            for (organization in fixture.organizations) {
                val result = graph.reviews.poll(graph.source.deliveryAccount(), organization.partition(fixture))
                assertEquals("The actual unresolved REVIEW was not looked up", 1, result.polled)
                assertEquals(0, result.resolved); assertEquals(1, result.unresolved); assertEquals(0, result.failed)
                val item = graph.status.detail(graph.attempt(organization), graph.entry(organization).operation.operationId,
                    graph.entry(organization).operation.payloadDigest).item
                assertEquals(EnterpriseSyncStatus.NEEDS_REVIEW, item.status); assertNull(item.resolution)
            }
            assertEquals("A result lookup resent final operations", 2, graph.operationRequests.get())
            assertEquals(2, graph.reviewRequests.get())
            graph.assertProtectedSurfaces()
            assertTrue("Stage marker must be durable before process termination",
                graph.preferences.edit().putInt(STAGE_PID, Process.myPid()).commit())
        } finally { graph.close() }
    }

    @Test
    fun recoverDecisionsAfterProcessDeathAndLostAnswer(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            val previous = graph.preferences.getInt(STAGE_PID, 0)
            assertTrue("The seed method must run in an earlier process", previous > 0 && previous != Process.myPid())
            assertTrue("The original Android signing key did not survive", graph.deviceKeys.hasKey())
            val (a, b) = fixture.organizations
            for (organization in fixture.organizations) {
                val entry = graph.entry(organization)
                assertTrue("Process death resolved or lost the REVIEW", entry.unresolvedReview)
                assertEquals(graph.preferences.getString(ACK_PREFIX + organization.id, null), hex(entry.acknowledgement!!.canonicalBytes))
            }
            val runner = EnterpriseOperationWorkRunner(graph.source, graph.persistence::discover, graph.service::dispatch, graph.reviews::poll)
            val factoryCalls = AtomicInteger()
            fun worker(): EnterpriseOperationWorker = TestListenableWorkerBuilder<EnterpriseOperationWorker>(context)
                .setInputData(Data.EMPTY).setWorkerFactory(object : WorkerFactory() {
                    override fun createWorker(appContext: Context, workerClassName: String, parameters: WorkerParameters): ListenableWorker? {
                        if (workerClassName != EnterpriseOperationWorker::class.java.name) return null
                        factoryCalls.incrementAndGet()
                        return EnterpriseOperationWorker(appContext, parameters, runner)
                    }
                }).build()
            val first = worker()
            assertTrue("System work input contained private selectors", first.inputData.keyValueMap.isEmpty())
            assertEquals("Review lookups must not make the delivery worker retry", ListenableWorker.Result.success(),
                withTimeout(90_000) { first.doWork() })
            val lost = graph.entry(a)
            assertTrue("A discarded answer became a local decision", lost.unresolvedReview)
            assertEquals(1L, lost.reviewRetry.attempts)
            assertEquals("Default equal-jitter review retry changed", 15_000L, lost.reviewRetry.delayMillis)
            val refused = graph.entry(b)
            val refusal = refused.resolution ?: throw AssertionError("The independent refusal did not arrive")
            assertEquals(ReviewResolution.REFUSED, refusal.snapshot().outcome)
            assertEquals("Synthetic native reviewer refusal 😀", refusal.snapshot().reason)
            assertEquals(graph.preferences.getString(ACK_PREFIX + b.id, null), hex(refused.acknowledgement!!.canonicalBytes))
            assertTrue(refused.reviewRetry.initial)
            assertEquals("Final REVIEW work was resent", 0, graph.operationRequests.get())
            val now = graph.clock.read()
            val recorded = lost.reviewRetry.clock ?: throw AssertionError("Durable review retry clock was not retained")
            val remaining = maxOf(0, lost.reviewRetry.delayMillis - (now.elapsedMillis - recorded.elapsedMillis))
            if (remaining > 0) delay(remaining + 50)
            val beforeSecond = graph.reviewRequests.get()
            assertEquals(ListenableWorker.Result.success(), withTimeout(90_000) { worker().doWork() })
            assertEquals("Only the unresolved partition should be looked up again", beforeSecond + 1, graph.reviewRequests.get())
            val accepted = graph.entry(a)
            assertEquals(ReviewResolution.ACCEPTED, accepted.resolution?.snapshot()?.outcome)
            assertEquals(graph.preferences.getString(ACK_PREFIX + a.id, null), hex(accepted.acknowledgement!!.canonicalBytes))
            val acceptedItem = graph.status.detail(graph.attempt(a), accepted.operation.operationId, accepted.operation.payloadDigest).item
            assertEquals(EnterpriseSyncStatus.ACCEPTED, acceptedItem.status)
            assertEquals(SyncResult.ACCEPTED_FOR_REVIEW, acceptedItem.receipt!!.result)
            assertTrue(EnterpriseSyncProjection.history(acceptedItem).any { it.kind == EnterpriseSyncHistoryKind.REVIEW_RESOLUTION })
            val refusedItem = graph.status.detail(graph.attempt(b), refused.operation.operationId, refused.operation.payloadDigest).item
            assertEquals(EnterpriseSyncStatus.REJECTED, refusedItem.status)
            try {
                graph.status.dismissRejected(graph.attempt(b), refused.operation.operationId, refused.operation.payloadDigest,
                    refusedItem.receipt!!.receiptDigest)
                fail("The original REVIEW fingerprint hid a later decision")
            } catch (_: EnterprisePersistenceUnavailable) { }
            assertTrue(graph.status.dismissRejected(graph.attempt(b), refused.operation.operationId, refused.operation.payloadDigest,
                refusedItem.resolution!!.resolutionDigest))
            assertTrue(graph.entry(b).dismissedRejected)
            val settled = graph.reviewRequests.get()
            assertEquals(ListenableWorker.Result.success(), withTimeout(90_000) { worker().doWork() })
            assertEquals("A resolved queue looked up results again", settled, graph.reviewRequests.get())
            assertEquals(3, factoryCalls.get())
            assertEquals(0, graph.operationRequests.get())
            graph.assertProtectedSurfaces()
        } finally {
            graph.cleanup()
            graph.close()
        }
    }

    private fun fixtureFile() = File(context.noBackupFilesDir, "enterprise-native-review-fixture.json")

    private class Organization(val id: String, val member: String, val asset: String, val meter: String, val segment: String) {
        fun partition(fixture: SourceFixture) = EnterprisePartition.of(fixture.session.accountId, fixture.origin, id)
        override fun toString(): String = "ReviewFixtureOrganization(redacted)"
    }
    private class SourceFixture(val origin: String, val certificate: String, val session: PortableAuth.Session,
        val type: String, val organizations: List<Organization>) {
        override fun toString(): String = "ReviewSourceFixture(redacted)"
    }

    private class Graph(private val context: Context, private val fixture: SourceFixture, seed: Boolean) : Closeable {
        val preferences = context.getSharedPreferences("enterprise-native-review", Context.MODE_PRIVATE)
        private val fence = EnterpriseSessionFence()
        private val platformKeys = AndroidKeystoreKeys()
        private val sessions = EnterpriseFencedSessionStore(KeystoreSessionStore(preferences, platformKeys, SESSION_ALIAS), fence)
        private val accountKeys = KeystoreAccountKey(preferences, platformKeys, BACKUP_ALIAS)
        init {
            if (seed) {
                assertNull("Unexpected existing native review session", sessions.load())
                sessions.save(fixture.session)
            }
            val current = sessions.load() ?: throw AssertionError("Retained native review session is unavailable")
            assertTrue("Retained source changed ownership", current.accountId == fixture.session.accountId && current.familyId == fixture.session.familyId)
        }
        private val http = fixtureHttp(fixture.certificate)
        private val auth = AuthFlow(AuthPorts(sessions, accountKeys, AuthWallClock { System.currentTimeMillis() / 1000 },
            PortableNonceSource { UUID.randomUUID().toString() }, AccountHttpApi(http, fixture.origin)))
        val deviceKeys = AndroidAnonymousDeviceKeys(auth, ApiEnvironment.PREVIEW)
        private val proof = PhotoProofTransport(deviceKeys, AnonymousProofHttpClient(http, fixture.origin), auth, fixture.origin, http)
        private val actualSigner = EnterpriseIdentityV1ContactSigner(proof)
        val operationRequests = AtomicInteger()
        val reviewRequests = AtomicInteger()
        private val signer = EnterpriseDeviceSigner { binding, path, bytes ->
            if (path.endsWith("/sync/operations")) operationRequests.incrementAndGet()
            if (path.endsWith("/sync/review-results")) reviewRequests.incrementAndGet()
            actualSigner.headers(binding, path, bytes)
        }
        private val contact = EnterpriseDeviceContactClient(http, fixture.origin, sessions, auth, fence, signer)
        private val files = EnterpriseModule.vaultFiles(context)
        private val keys = EnterpriseModule.vaultKeys()
        private val erasure = EnterpriseVaultErasure(files, keys, fence)
        val clock = AndroidEnterpriseDeviceClock(context)
        val persistence = ProtectedEnterprisePersistence(files, keys, fence, erasure,
            { sessions.load()?.let { EnterpriseLocalIdentity(it.accountId, fixture.origin, it.familyId) } }, clock::read, contact)
        private val queue = ProtectedEnterpriseQueue(persistence)
        val status = ProtectedEnterpriseSyncStatusStore(persistence)
        val source = EnterpriseOperationSessionSource(fixture.origin, sessions, auth, fence, persistence::protectedEnrollment)
        private val activity = EnterpriseDeliveryActivity()
        val service = EnterpriseQueueService(source, queue, EnterpriseOperationHttpClient(http, source, signer),
            EnterpriseQueueRegistry(Producer(fixture)), clock::read, entropy = { 0L }, activity = activity)
        // Deterministic half-delay within the unchanged30s/6h review retry defaults.
        val reviews = EnterpriseReviewService(source, queue, EnterpriseReviewHttpClient(http, source, signer), clock::read,
            entropy = { 0L }, activity = activity)

        suspend fun attempt(organization: Organization): EnterpriseQueueAttempt = source.attempt(source.account(), organization.partition(fixture))

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

        suspend fun operation(organization: Organization): EnterpriseOperation {
            val partition = organization.partition(fixture)
            val enrolled = attempt(organization)
            val raw = JSONObject().put("schema_version", 1).put("operation_id", UUID.randomUUID().toString())
                .put("owner_account_id", fixture.session.accountId).put("api_origin", fixture.origin).put("org_id", organization.id)
                .put("membership_id", organization.member).put("enrollment_id", enrolled.enrollment.enrollmentId)
                .put("type", fixture.type).put("operation_schema", 1)
                .put("entity", JSONObject().put("type", "meter_reading").put("id", UUID.randomUUID().toString()).put("expected_revision", JSONObject.NULL))
                .put("occurred_at", EnterpriseTime.formatInstant(Instant.now())).put("entry_zone", "UTC").put("device_offset_seconds", 0)
                .put("depends_on", JSONArray()).put("soft_references", JSONArray()).put("attachments", JSONArray())
                .put("payload", JSONObject().put("asset_id", organization.asset).put("meter_id", organization.meter)
                    .put("segment_id", organization.segment).put("kind", "ODOMETER").put("value", "7").put("method", "TYPED"))
            return EnterpriseOperation.seal(raw.toString().toByteArray(), SyncBinding.of(partition.ownerAccount, partition.origin, partition.organizationId))
        }

        suspend fun entry(organization: Organization): EnterpriseQueueEntry {
            val attempt = attempt(organization)
            val row = persistence.record(organization.partition(fixture), organization.id)
                ?: throw AssertionError("Protected local projection is unavailable")
            val id = try { JSONObject(String(row.envelope, Charsets.UTF_8)).getString("operation_id") }
                catch (_: Exception) { throw AssertionError("Protected local projection is malformed") }
            return queue.entry(attempt, EnterprisePartition.requireId(id)) ?: throw AssertionError("Original typed operation is unavailable")
        }

        suspend fun assertProtectedSurfaces() {
            for (organization in fixture.organizations) {
                val token = EnterpriseVaultCodec.token(organization.partition(fixture))
                val generations = files.generations(token)
                assertTrue("Committed generations were not retained", generations.isNotEmpty())
                for (generation in generations) {
                    for (descriptor in listOf(false, true)) {
                        val bytes = files.readGeneration(token, generation, descriptor) ?: continue
                        val text = String(bytes, Charsets.ISO_8859_1)
                        assertFalse("Private decision appeared outside encryption", text.contains("Synthetic native reviewer"))
                        assertFalse(text.contains(fixture.session.accountId))
                        assertFalse(text.contains(fixture.session.accessToken))
                        assertFalse(text.contains(organization.asset))
                    }
                }
                keys.aliases(EnterpriseVaultGenerationNames.prefix(token)).forEach { alias ->
                    assertNull("Vault key material was exportable", keys.existing(alias)!!.secret.encoded)
                }
            }
        }

        fun cleanup() {
            fixture.organizations.forEach { assertTrue("Native review cleanup was incomplete", erasure.partitionRevoked(it.partition(fixture))) }
            val identityAlias = AndroidAnonymousDeviceKeys.aliasFor(AuthFlow.AuthState.Active(fixture.session), fixture.origin)
            KeyStore.getInstance("AndroidKeyStore").apply {
                load(null); deleteEntry(identityAlias); deleteEntry(SESSION_ALIAS); deleteEntry(BACKUP_ALIAS)
            }
            assertTrue(preferences.edit().clear().commit())
            val privateFixture = File(context.noBackupFilesDir, "enterprise-native-review-fixture.json")
            assertTrue("Private native proof cleanup failed", privateFixture.delete())
            assertFalse("Private native proof survived cleanup", privateFixture.exists())
        }

        override fun close() {
            http.connectionPool.evictAll()
            http.dispatcher.executorService.shutdown()
        }
    }

    /** Named synthetic Meter projector. It exists only in the test APK and never decides review outcomes. */
    private class Producer(private val fixture: SourceFixture) : EnterpriseQueueProducer {
        override fun descriptor() = EnterpriseQueueProducerDescriptor(fixture.type)
        override fun save(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation) {
            attempt.assertCurrent()
            val organization = fixture.organizations.single { it.id == attempt.partition.organizationId }
            require(operation.type == "meter.reading.record" && operation.membershipId == organization.member &&
                operation.enrollmentId == attempt.enrollment.enrollmentId && tx.record(organization.id) == null)
            tx.put(EnterpriseLocalRecord(attempt.partition, organization.id,
                JSONObject().put("operation_id", operation.operationId).put("status", "SAVED").toString().toByteArray()))
        }
        override fun acknowledge(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
            ack: OperationAcknowledgement) {
            attempt.assertCurrent()
            require(ack.matches(operation) && ack.result == SyncResult.ACCEPTED_FOR_REVIEW)
            val organization = fixture.organizations.single { it.id == attempt.partition.organizationId }
            // Receipt only: held field work never becomes a local accepted projection.
            tx.put(EnterpriseLocalRecord(attempt.partition, organization.id,
                JSONObject().put("operation_id", operation.operationId).put("status", "HELD").toString().toByteArray()))
        }
    }

    companion object {
        private const val STAGE_PID = "seed_process"
        private const val ACK_PREFIX = "original_ack_"
        private const val SESSION_ALIAS = "enterprise_native_review_session"
        private const val BACKUP_ALIAS = "enterprise_native_review_backup"
        private fun hex(bytes: ByteArray): String = MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it.toInt() and 255) } +
            ":" + bytes.size

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
            val type = value.getString("operation_type").also { require(it == "meter.reading.record") }
            val organizations = value.getJSONArray("organizations").also { require(it.length() == 2) }.let { rows ->
                (0 until rows.length()).map { index -> rows.getJSONObject(index).let {
                    Organization(EnterprisePartition.requireId(it.getString("org_id")), EnterprisePartition.requireId(it.getString("membership_id")),
                        EnterprisePartition.requireId(it.getString("asset_id")), EnterprisePartition.requireId(it.getString("meter_id")),
                        EnterprisePartition.requireId(it.getString("segment_id")))
                } }
            }
            require(organizations.map { it.id }.distinct().size == 2)
            SourceFixture(origin, certificate, session, type, organizations)
        } catch (_: Exception) { throw AssertionError("Disposable review fixture is invalid") }

        private fun fixtureHttp(pem: String): OkHttpClient = try {
            val certificate = CertificateFactory.getInstance("X.509").generateCertificate(pem.byteInputStream())
            val anchors = KeyStore.getInstance(KeyStore.getDefaultType()).apply { load(null); setCertificateEntry("disposable-review", certificate) }
            val trust = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply { init(anchors) }
                .trustManagers.single() as X509TrustManager
            val tls = SSLContext.getInstance("TLS").apply { init(null, arrayOf(trust), null) }
            OkHttpClient.Builder().sslSocketFactory(tls.socketFactory, trust).dns(object : Dns {
                override fun lookup(hostname: String): List<InetAddress> {
                    check(hostname == "api.anpfuel.example.invalid") { "Native review refused an unrelated host" }
                    return listOf(InetAddress.getByAddress(byteArrayOf(127, 0, 0, 1)))
                }
            }).followRedirects(false).followSslRedirects(false).retryOnConnectionFailure(false).build()
        } catch (_: Exception) { throw AssertionError("Disposable review trust anchor is invalid") }
    }
}
