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
import com.anpfuel.application.portable.*
import com.anpfuel.data.di.EnterpriseModule
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
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
import com.anpfuel.domain.enterprise.EnterprisePendingOperation
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.sync.*
import com.anpfuel.domain.portable.PortableAuth
import java.io.Closeable
import java.io.File
import java.net.InetAddress
import java.security.KeyStore
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
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/** Opt-in two-process acceptance against the private ordinary PostGIS fixture.
 * Run the two named methods separately, force-stop the owned target between
 * them, and never clear app data. No test producer enters the product registry.
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseOperationQueueDeviceTest {
    private val context: Context get() = InstrumentationRegistry.getInstrumentation().targetContext

    @Test
    fun seedLostAnswerAndIndependentOtherOrg(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = true)
        try {
            assertEquals("Unexpected previous native queue stage", 0, graph.preferences.getInt(STAGE_PID, 0))
            graph.establishCustody()
            val first = graph.operation(fixture.organizations[0], "Synthetic native durable fact A")
            val second = graph.operation(fixture.organizations[1], "Synthetic native durable fact B")
            graph.persistence.putPrivateFile(firstPartition(fixture), first.entity.reference.id, ATTACHMENT)
            assertTrue(graph.persistence.enqueue(EnterprisePendingOperation(firstPartition(fixture), UUID.randomUUID().toString(),
                first.entity.reference.id, first.canonicalBytes)))
            assertTrue(graph.service.save(firstPartition(fixture), first))
            assertTrue(graph.service.save(secondPartition(fixture), second))
            assertFalse("Identical local save reapplied its projector", graph.service.save(firstPartition(fixture), first))
            assertEquals(2, graph.producer.saveCalls.get())
            val lost = graph.service.dispatch(firstPartition(fixture))
            assertEquals("The actual source must discard its first committed answer", 1, lost.retryable)
            assertEquals(0, lost.completed)
            val pending = graph.entry(fixture.organizations[0])
            assertNull("Transport loss became local acceptance", pending.acknowledgement)
            assertTrue("Lost answer did not leave durable retry state", pending.retry.attempts == 1L)
            assertTrue("Default equal-jitter retry was changed", pending.retry.delayMillis == 15_000L)
            assertTrue("Immutable original intent changed after loss", pending.operation.canonicalBytes.contentEquals(first.canonicalBytes))
            graph.assertProjection(fixture.organizations[0], "SAVED", ackCount = 0)
            val independent = graph.service.dispatch(secondPartition(fixture))
            assertEquals("A lost answer held another organization", 1, independent.completed)
            assertEquals(0, independent.retryable)
            graph.assertProjection(fixture.organizations[1], "ACCEPTED", ackCount = 1)
            assertEquals(2, graph.operationRequests.get())
            assertEquals(1, graph.producer.ackCalls.get())
            graph.assertProtectedSurfaces()
            assertTrue("Stage marker must be durable before process termination",
                graph.preferences.edit().putInt(STAGE_PID, Process.myPid()).commit())
        } finally {
            // Deliberately retain ciphertext, real Keystore handles and the
            // current encrypted session for the separate process-death method.
            graph.close()
        }
    }

    @Test
    fun recoverAfterProcessDeath(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            val previous = graph.preferences.getInt(STAGE_PID, 0)
            assertTrue("The seed method must run in an earlier process", previous > 0 && previous != Process.myPid())
            assertTrue("The original Android signing key did not survive", graph.deviceKeys.hasKey())
            val a = graph.entry(fixture.organizations[0])
            val b = graph.entry(fixture.organizations[1])
            assertNull(a.acknowledgement)
            val retainedB = b.acknowledgement?.canonicalBytes ?: throw AssertionError("Independent final receipt was not retained")
            graph.assertProjection(fixture.organizations[0], "SAVED", ackCount = 0)
            graph.assertProjection(fixture.organizations[1], "ACCEPTED", ackCount = 1)
            val now = graph.clock.read()
            val recordedClock = a.retry.clock ?: throw AssertionError("Durable retry clock was not retained")
            assertTrue("Process death must not invent a reboot", recordedClock.boot == now.boot && now.elapsedMillis >= recordedClock.elapsedMillis)
            val remaining = maxOf(0, a.retry.delayMillis - (now.elapsedMillis - recordedClock.elapsedMillis))
            assertTrue("Acceptance never waits an invented retry policy", remaining <= 30_000)
            if (remaining > 0) delay(remaining + 20)
            val runner = EnterpriseOperationWorkRunner(graph.source, graph.persistence::discover, graph.service::dispatch)
            val factoryCalls = AtomicInteger()
            fun worker(): EnterpriseOperationWorker = TestListenableWorkerBuilder<EnterpriseOperationWorker>(context)
                .setInputData(Data.EMPTY).setWorkerFactory(object : WorkerFactory() {
                    override fun createWorker(appContext: Context, workerClassName: String, parameters: WorkerParameters): ListenableWorker? {
                        if (workerClassName != EnterpriseOperationWorker::class.java.name) return null
                        factoryCalls.incrementAndGet()
                        return EnterpriseOperationWorker(appContext, parameters, runner)
                    }
                }).build()
            val recoveredWorker = worker()
            assertTrue("System work input contained private selectors", recoveredWorker.inputData.keyValueMap.isEmpty())
            assertEquals(ListenableWorker.Result.success(), withTimeout(90_000) { recoveredWorker.doWork() })
            assertEquals(1, factoryCalls.get())
            val accepted = graph.entry(fixture.organizations[0])
            assertTrue("Lost answer did not converge on the actual final receipt", accepted.acknowledgement?.result == SyncResult.ACCEPTED)
            assertTrue("Restart changed the original immutable operation", accepted.operation.canonicalBytes.contentEquals(a.operation.canonicalBytes))
            assertTrue("Another partition's original receipt changed", retainedB.contentEquals(graph.entry(fixture.organizations[1]).acknowledgement!!.canonicalBytes))
            graph.assertProjection(fixture.organizations[0], "ACCEPTED", ackCount = 1)
            graph.assertProjection(fixture.organizations[1], "ACCEPTED", ackCount = 1)
            assertEquals("Only the pending original scope should be resent", 1, graph.operationRequests.get())
            assertEquals("Recovered ACK projection must be applied once", 1, graph.producer.ackCalls.get())
            assertEquals(0, graph.producer.saveCalls.get())
            assertEquals(ListenableWorker.Result.success(), withTimeout(90_000) { worker().doWork() })
            assertEquals(2, factoryCalls.get())
            assertEquals("A finished queue resent acknowledged work", 1, graph.operationRequests.get())
            assertEquals("A repeated worker reapplied the ACK projection", 1, graph.producer.ackCalls.get())
            graph.assertProtectedSurfaces()
        } finally {
            // Assertions precede cleanup. Only the two fixture partitions and
            // this synthetic account's exact test aliases/preferences are owned.
            graph.cleanup()
            graph.close()
        }
    }

    @Test
    fun fixtureInputAndTrustFailuresStayRedacted() {
        val temporary = File(context.cacheDir, "queue-parser-" + UUID.randomUUID())
        val sentinel = "SYNTHETIC-QUEUE-CREDENTIAL-MUST-NOT-BE-LOGGED"
        try {
            for (raw in listOf("{\"access_token\":\"$sentinel\",", "{\"origin\":\"$sentinel\"}")) {
                temporary.writeText(raw)
                try { readFixture(temporary); fail("Malformed private fixture was accepted") }
                catch (failure: AssertionError) {
                    assertEquals("Disposable queue fixture is invalid", failure.message)
                    assertNull(failure.cause)
                    assertFalse(failure.toString().contains(sentinel))
                }
            }
            try { fixtureHttp(sentinel); fail("Malformed source trust was accepted") }
            catch (failure: AssertionError) {
                assertEquals("Disposable queue trust anchor is invalid", failure.message)
                assertNull(failure.cause)
            }
        } finally { temporary.delete() }
    }

    @Test
    fun actualScopedKeyDiscoveryIncludesRetainedAndDeniedMarkers() {
        val sharedOrganization = UUID.randomUUID().toString()
        val partitions = List(2) { EnterprisePartition.of(UUID.randomUUID().toString(), ApiEnvironment.PREVIEW.origin, sharedOrganization) }
        val keys = AndroidEnterpriseVaultKeys()
        val owned = mutableListOf<String>()
        try {
            val aliases = partitions.map { partition ->
                val token = EnterpriseVaultCodec.token(partition)
                listOf(EnterpriseVaultGenerationNames.data(token, 1), EnterpriseVaultGenerationNames.denied(token),
                    EnterpriseVaultGenerationNames.retained(token)).also { values ->
                    for (alias in values) {
                        assertNull("Unexpected synthetic key collision", keys.existing(alias))
                        owned += alias
                        assertNull("Real Android vault key material was exportable", keys.create(alias, null).secret.encoded)
                    }
                }
            }
            for ((index, partition) in partitions.withIndex()) {
                val tag = EnterpriseVaultCodec.accountTag(partition.ownerAccount, partition.origin)
                val prefixes = listOf(EnterpriseVaultGenerationNames.PREFIX + tag + "-",
                    EnterpriseVaultGenerationNames.PREFIX + "denied_" + tag + "-",
                    EnterpriseVaultGenerationNames.PREFIX + "lock_" + tag + "-")
                for (kind in prefixes.indices) {
                    val actual = try { keys.aliases(prefixes[kind]) }
                    catch (_: Exception) { throw AssertionError("Real provider refused a valid current-account discovery prefix") }
                    assertTrue("Scoped Android discovery omitted its marker or exposed another account", actual == listOf(aliases[index][kind]))
                }
            }
            val tag = EnterpriseVaultCodec.accountTag(partitions[0].ownerAccount, partitions[0].origin)
            for (invalid in listOf("unrelated_", EnterpriseVaultGenerationNames.PREFIX + "denied_",
                EnterpriseVaultGenerationNames.PREFIX + "lock_", EnterpriseVaultGenerationNames.PREFIX + "denied_" + tag,
                EnterpriseVaultGenerationNames.PREFIX + "lock_" + tag + "-partial")) {
                try { keys.aliases(invalid); fail("Real provider accepted an unscoped or partial discovery prefix") }
                catch (_: IllegalArgumentException) { /* Exact reviewed prefixes only. */ }
            }
        } finally {
            var incomplete = false
            for (alias in owned) {
                try { keys.delete(alias) } catch (_: Exception) { incomplete = true }
            }
            assertFalse("Synthetic scoped-key cleanup was incomplete", incomplete)
        }
    }

    private fun fixtureFile() = File(context.noBackupFilesDir, "enterprise-native-queue-fixture.json")
    private fun firstPartition(fixture: SourceFixture) = fixture.organizations[0].partition(fixture)
    private fun secondPartition(fixture: SourceFixture) = fixture.organizations[1].partition(fixture)

    private class Organization(val id: String, val member: String, val target: String) {
        fun partition(fixture: SourceFixture) = EnterprisePartition.of(fixture.session.accountId, fixture.origin, id)
        override fun toString(): String = "QueueFixtureOrganization(redacted)"
    }
    private class SourceFixture(val origin: String, val certificate: String, val session: PortableAuth.Session,
        val type: String, val organizations: List<Organization>) {
        override fun toString(): String = "QueueSourceFixture(redacted)"
    }

    private class Graph(private val context: Context, private val fixture: SourceFixture, seed: Boolean) : Closeable {
        val preferences = context.getSharedPreferences("enterprise-native-queue", Context.MODE_PRIVATE)
        private val fence = EnterpriseSessionFence()
        private val platformKeys = AndroidKeystoreKeys()
        private val sessions = EnterpriseFencedSessionStore(KeystoreSessionStore(preferences, platformKeys, SESSION_ALIAS), fence)
        private val accountKeys = KeystoreAccountKey(preferences, platformKeys, BACKUP_ALIAS)
        init {
            if (seed) {
                assertNull("Unexpected existing native queue session", sessions.load())
                sessions.save(fixture.session)
            }
            val current = sessions.load() ?: throw AssertionError("Retained native queue session is unavailable")
            assertTrue("Retained source changed ownership", current.accountId == fixture.session.accountId && current.familyId == fixture.session.familyId)
        }
        private val http = fixtureHttp(fixture.certificate)
        private val auth = AuthFlow(AuthPorts(sessions, accountKeys, AuthWallClock { System.currentTimeMillis() / 1000 },
            PortableNonceSource { UUID.randomUUID().toString() }, AccountHttpApi(http, fixture.origin)))
        val deviceKeys = AndroidAnonymousDeviceKeys(auth, ApiEnvironment.PREVIEW)
        private val proof = PhotoProofTransport(deviceKeys, AnonymousProofHttpClient(http, fixture.origin), auth, fixture.origin, http)
        private val actualSigner = EnterpriseIdentityV1ContactSigner(proof)
        val operationRequests = AtomicInteger()
        private val signer = EnterpriseDeviceSigner { binding, path, bytes ->
            if (path.endsWith("/sync/operations")) operationRequests.incrementAndGet()
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
        val source = EnterpriseOperationSessionSource(fixture.origin, sessions, auth, fence, persistence::protectedEnrollment)
        val producer = Producer(fixture)
        private val transport = EnterpriseOperationHttpClient(http, source, signer)
        val service = EnterpriseQueueService(source, queue, transport, EnterpriseQueueRegistry(producer), clock::read,
            entropy = { 0L }) // Deterministic half-delay within the unchanged30s/6h defaults.
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
            val alias = AndroidAnonymousDeviceKeys.aliasFor(AuthFlow.AuthState.Active(fixture.session), fixture.origin)
            val privateKey = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }.getKey(alias, null)
            assertNotNull("Actual Android signing key is missing", privateKey)
            assertNull("Android private signing material was exportable", privateKey.encoded)
        }

        suspend fun operation(organization: Organization, value: String): EnterpriseOperation {
            val partition = organization.partition(fixture)
            val attempt = source.attempt(source.account(), partition)
            val raw = JSONObject().put("schema_version", 1).put("operation_id", UUID.randomUUID().toString())
                .put("owner_account_id", fixture.session.accountId).put("api_origin", fixture.origin).put("org_id", organization.id)
                .put("membership_id", organization.member).put("enrollment_id", attempt.enrollment.enrollmentId)
                .put("type", fixture.type).put("operation_schema", 1)
                .put("entity", JSONObject().put("type", "setting").put("id", organization.target).put("expected_revision", JSONObject.NULL))
                .put("occurred_at", EnterpriseTime.formatInstant(Instant.now())).put("entry_zone", "UTC").put("device_offset_seconds", 0)
                .put("depends_on", org.json.JSONArray()).put("soft_references", org.json.JSONArray()).put("attachments", org.json.JSONArray())
                .put("payload", JSONObject().put("value", value))
            return EnterpriseOperation.seal(raw.toString().toByteArray(), SyncBinding.of(partition.ownerAccount, partition.origin, partition.organizationId))
        }

        suspend fun entry(organization: Organization): EnterpriseQueueEntry {
            val partition = organization.partition(fixture)
            val local = record(organization)
            val attempt = source.attempt(source.account(), partition)
            val entry = queue.entry(attempt, EnterprisePartition.requireId(local.getString("operation_id")))
                ?: throw AssertionError("Original typed operation is unavailable")
            assertTrue("Original canonical bytes did not survive", hex(entry.operation.canonicalBytes) == local.getString("operation_hex"))
            return entry
        }

        private suspend fun record(organization: Organization): JSONObject = try {
            val row = persistence.record(organization.partition(fixture), organization.target)
                ?: throw AssertionError("Protected local projection is unavailable")
            JSONObject(String(row.envelope, Charsets.UTF_8))
        } catch (_: Exception) { throw AssertionError("Protected local projection is malformed") }

        suspend fun assertProjection(organization: Organization, status: String, ackCount: Int) {
            val row = record(organization)
            assertTrue("Protected status does not match the actual acknowledgement", row.getString("status") == status)
            assertEquals(1, row.getInt("save_count"))
            assertEquals(ackCount, row.getInt("ack_count"))
        }

        suspend fun assertProtectedSurfaces() {
            val first = fixture.organizations[0]
            val original = entry(first).operation
            val legacy = persistence.pending(first.partition(fixture), 50)
            assertEquals("Legacy opaque work was reinterpreted or deleted", 1, legacy.size)
            assertTrue(legacy.single().envelope.contentEquals(original.canonicalBytes))
            assertArrayEquals(ATTACHMENT, persistence.privateFile(first.partition(fixture), first.target))
            for (organization in fixture.organizations) {
                val p = organization.partition(fixture)
                val token = EnterpriseVaultCodec.token(p)
                val generations = files.generations(token)
                assertTrue("Committed generations were not retained", generations.isNotEmpty())
                for (generation in generations) {
                    for (descriptor in listOf(false, true)) {
                        val bytes = files.readGeneration(token, generation, descriptor) ?: continue
                        val text = String(bytes, Charsets.ISO_8859_1)
                        assertFalse("Private queue fact appeared outside encryption", text.contains("Synthetic native durable fact"))
                        assertFalse(text.contains(fixture.session.accountId))
                        assertFalse(text.contains(fixture.session.accessToken))
                    }
                }
                keys.aliases(EnterpriseVaultGenerationNames.prefix(token)).forEach { alias ->
                    assertNull("Vault key material was exportable", keys.existing(alias)!!.secret.encoded)
                }
            }
        }

        fun cleanup() {
            fixture.organizations.forEach { assertTrue("Native queue cleanup was incomplete", erasure.partitionRevoked(it.partition(fixture))) }
            val identityAlias = AndroidAnonymousDeviceKeys.aliasFor(AuthFlow.AuthState.Active(fixture.session), fixture.origin)
            KeyStore.getInstance("AndroidKeyStore").apply {
                load(null); deleteEntry(identityAlias); deleteEntry(SESSION_ALIAS); deleteEntry(BACKUP_ALIAS)
            }
            assertTrue(preferences.edit().clear().commit())
            val privateFixture = File(context.noBackupFilesDir, "enterprise-native-queue-fixture.json")
            assertTrue("Private native proof cleanup failed", privateFixture.delete())
            assertFalse("Private native proof survived cleanup", privateFixture.exists())
        }

        override fun close() {
            http.connectionPool.evictAll()
            http.dispatcher.executorService.shutdown()
        }
    }

    /** Named synthetic owning projector. This class exists only in the test APK. */
    private class Producer(private val fixture: SourceFixture) : EnterpriseQueueProducer {
        val saveCalls = AtomicInteger()
        val ackCalls = AtomicInteger()
        override fun descriptor() = EnterpriseQueueProducerDescriptor(fixture.type)
        override fun save(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation) {
            attempt.assertCurrent()
            val organization = fixture.organizations.single { it.id == attempt.partition.organizationId }
            require(operation.type == "validation.setting" && operation.operationSchema == 1L &&
                operation.entity.reference.id == organization.target && operation.membershipId == organization.member &&
                operation.enrollmentId == attempt.enrollment.enrollmentId && tx.record(organization.target) == null)
            saveCalls.incrementAndGet()
            val row = JSONObject().put("operation_id", operation.operationId).put("operation_hex", hex(operation.canonicalBytes))
                .put("status", "SAVED").put("save_count", 1).put("ack_count", 0)
            tx.put(EnterpriseLocalRecord(attempt.partition, organization.target, row.toString().toByteArray()))
        }
        override fun acknowledge(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
            ack: OperationAcknowledgement) {
            attempt.assertCurrent()
            require(ack.matches(operation) && ack.result == SyncResult.ACCEPTED)
            val current = tx.record(operation.entity.reference.id) ?: throw AssertionError("ACK projector lost its local record")
            val row = try { JSONObject(String(current.envelope, Charsets.UTF_8)) }
                catch (_: Exception) { throw AssertionError("ACK projector received malformed protected state") }
            ackCalls.incrementAndGet()
            // A real owning projector must preserve a newer local edit.
            if (row.getString("operation_id") != operation.operationId) return
            row.put("status", "ACCEPTED").put("ack_count", row.getInt("ack_count") + 1)
            tx.put(EnterpriseLocalRecord(attempt.partition, current.id, row.toString().toByteArray()))
        }
    }

    companion object {
        private const val STAGE_PID = "seed_process"
        private const val SESSION_ALIAS = "enterprise_native_queue_session"
        private const val BACKUP_ALIAS = "enterprise_native_queue_backup"
        private val ATTACHMENT = "SYNTHETIC-NATIVE-QUEUE-ATTACHMENT-PRESERVED".toByteArray()
        private fun hex(bytes: ByteArray): String = bytes.joinToString("") { "%02x".format(it.toInt() and 255) }

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
            val type = value.getString("operation_type").also { require(it == "validation.setting") }
            val organizations = value.getJSONArray("organizations").also { require(it.length() == 2) }.let { rows ->
                (0 until rows.length()).map { index -> rows.getJSONObject(index).let {
                    Organization(EnterprisePartition.requireId(it.getString("org_id")),
                        EnterprisePartition.requireId(it.getString("membership_id")), EnterprisePartition.requireId(it.getString("target_id")))
                } }
            }
            require(organizations.map { it.id }.distinct().size == 2)
            SourceFixture(origin, certificate, session, type, organizations)
        } catch (_: Exception) { throw AssertionError("Disposable queue fixture is invalid") }

        private fun fixtureHttp(pem: String): OkHttpClient = try {
            val certificate = CertificateFactory.getInstance("X.509").generateCertificate(pem.byteInputStream())
            val anchors = KeyStore.getInstance(KeyStore.getDefaultType()).apply { load(null); setCertificateEntry("disposable-queue", certificate) }
            val trust = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply { init(anchors) }
                .trustManagers.single() as X509TrustManager
            val tls = SSLContext.getInstance("TLS").apply { init(null, arrayOf(trust), null) }
            OkHttpClient.Builder().sslSocketFactory(tls.socketFactory, trust).dns(object : Dns {
                override fun lookup(hostname: String): List<InetAddress> {
                    check(hostname == "api.anpfuel.example.invalid") { "Native queue refused an unrelated host" }
                    return listOf(InetAddress.getByAddress(byteArrayOf(127, 0, 0, 1)))
                }
            }).followRedirects(false).followSslRedirects(false).retryOnConnectionFailure(false).build()
        } catch (_: Exception) { throw AssertionError("Disposable queue trust anchor is invalid") }
    }
}
